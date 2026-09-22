# Segurança técnica

**Status:** requisitos para implementação

**Referência geral:** OWASP ASVS 5 · [THREAT_MODEL.md](THREAT_MODEL.md)

## 1. Modelo de confiança

- Cloudflare reduz tráfego malicioso, mas não é fronteira única de autorização.
- Caddy encaminha requisições; a aplicação valida identidade, autorização e entrada.
- PostgreSQL não é acessível pela Internet.
- Stripe é autoridade do pagamento; o ledger local é autoridade do uso de INK.
- Nenhum dado vindo de browser, webhook, header de proxy ou job é confiável sem validação.

## 2. Autenticação

- Sessão com token aleatório de alta entropia enviado uma vez ao browser.
- Apenas hash do token é persistido.
- Cookie `HttpOnly`, `Secure`, `SameSite=Lax`, com escopo mínimo.
- Rotação após login, elevação de privilégio e alteração de credencial.
- Expiração absoluta e por inatividade; encerramento de outras sessões disponível.
- Senhas com Argon2id e parâmetros versionados, ajustados por benchmark.
- Tokens de confirmação e recuperação são de uso único, curtos e persistidos como hash.
- Administradores exigem segundo fator antes do beta público.
- Respostas de login e recuperação não revelam se um email existe.

Não implementar JWT como sessão principal no browser. JWT pode ser reavaliado para integração específica, não por conveniência.

### Segundo fator administrativo (P16-T05)

O mecanismo é TOTP nativo sobre RFC 4226/6238 (`internal/platform/mfa`), e a decisão está registrada na ADR-014: a biblioteca padrão do Go cobre todos os primitivos (HMAC, AES-GCM, comparação em tempo constante, `crypto/rand`), os vectores publicados pela RFC provam a correção do algoritmo em teste, e as partes que uma biblioteca genérica **não** decide — quanto skew de relógio é tolerado, que um passo aceito é gasto, sob que conta o segredo é selado — são exatamente as que aqui são política escrita. A superfície é `internal/identity` (casos de uso) sobre esse mecanismo por porta.

O que existe e por quê:

- **o segredo é selado, nunca hasheado.** Verificar um código exige o segredo em claro, então ele é cifrado com AES-256-GCM e o identificador da conta é o *additional authenticated data*: um valor selado copiado de outra linha não abre. A coluna guarda só criptografia, então um dump, uma réplica ou um backup não contêm um segundo fator utilizável. A chave (32 bytes) entra com a composição dos módulos, pelo mesmo critério que a T03 registrou para proxies confiáveis e a T04 para o Turnstile;
- **um passo de tempo é de uso único.** O maior passo aceito vive na própria linha (`app.account_mfa.last_accepted_step`) e o avanço é uma única instrução (`WHERE last_accepted_step < $2`), então duas verificações concorrentes do mesmo código não podem ambas vencer. Um código dentro da janela que já foi gasto é recusado como `mfa_code_replayed`, separado de `mfa_code_invalid`: um é replay de um código correto, o outro é um palpite errado, e quem lê um incidente precisa distinguir os dois;
- **skew limitado.** Um passo para cada lado é o default (tolerância a relógio dessincronizado por segundos); alargar a janela amplia a superfície de palpite e é decisão, não inferência. `Verify` caminha a janela inteira e compara em tempo constante, sem parar cedo de forma dependente do conteúdo;
- **códigos de recuperação são de uso único e hasheados** com o mesmo Argon2id das senhas, e a forma do código é validada **antes** de qualquer hashing, para que o endpoint não seja oráculo nem gasto barato de CPU. A recusa de um código já gasto é a mesma de um código inexistente, pelo mesmo motivo da T04;
- **elevação é propriedade da sessão**, não da conta: `app.sessions.mfa_verified_at` diz quando **aquela** sessão apresentou o fator. A conta pode ter matrícula confirmada e ainda assim uma sessão que nunca apresentou nada, e é essa sessão que o gate administrativo recusa;
- **recuperação falha fechado.** A ordem é consumir o código, registrar o fato na trilha de auditoria e só então elevar a sessão. Uma recuperação que não pode ser atribuída **não** eleva ninguém (o código é gasto e o acesso não é concedido), que é a única direção que não pode ser abusada por repetição até a trilha ficar disponível. Os fatos registrados são `mfa.enrolled` e `mfa.backup_code_used`.

O gate administrativo (`internal/moderation/adapters/http`) exige, além do papel, que a sessão tenha apresentado o fator dentro de `domain.StepUpWindow` (15 minutos) e recusa com `mfa_step_up_required` nas três rotas de triagem (fila, claim e decisão). A recusa não descreve o estado da matrícula: ela diz que falta um step-up, nunca se a conta tem fator, se ele está confirmado ou quando foi apresentado. As rotas de step-up e recuperação carregam o orçamento `mfa.verify` (endereço e conta) da política da T03, porque um código é um segredo adivinhável.

Limites conhecidos, que precisam de decisão antes do beta:

- **matrícula e confirmação não têm orçamento por ação.** Elas só são alcançáveis por sessão autenticada, cada chamada reescreve no máximo uma linha pendente e não há amplificação (nem envio de email); o custo é limitado pela camada de plataforma da T02. Se a fase 18 expuser o widget, vale revisitar: um oráculo de geração de segredo por conta é barato de limitar;
- **a memória de passos gastos é a linha do banco**, então — ao contrário do que acontece com os tokens do Turnstile — ela é global e não por processo: duas instâncias respeitam o mesmo passo;
- **o segredo é mostrado uma única vez**, na resposta de `POST /api/v1/me/mfa/enrollment`, e os códigos de recuperação também só existem em claro na resposta de confirmação; nenhuma outra resposta os repete (há teste que percorre as respostas).

## 3. Autorização

- Toda ação possui verificação server-side de ator, recurso e permissão.
- IDs enviados pelo cliente nunca provam ownership.
- Rotas administrativas usam namespace, middleware e auditoria próprios.
- Contas pagantes não recebem bypass de regras.
- Acesso direto ao banco usa papel sem permissão de migration ou superuser.

### Bootstrap do primeiro administrador (P19-T09)

Uma instalação nasce sem administrador, e é a promoção do primeiro que a cria. Esse ato é o único do produto que **não** é um pedido HTTP: acontece na linha de comando, em `arena admin bootstrap`, por quem já detém o DSN — e o processo que o executa não serve página nenhuma. A superfície de moderação continua sem rota que conceda papel, e um teste percorre a árvore para reprovar qualquer adaptador HTTP ou HTML que **nomeie** o caso de uso, então ligar o comando a uma rota não é um descuido possível: é um teste vermelho.

As regras da promoção fecham buracos distintos, e nenhuma delas é decorativa:

- **a conta precisa existir e ter endereço verificado.** Um endereço digitado errado, ou um cadastro que nunca foi confirmado, não promove ninguém;
- **a conta precisa poder autenticar.** Suspensa ou excluída mantém o endereço verificado, então sem esta regra a instalação ganharia um administrador que não consegue entrar — e que bloquearia o bootstrap sem conceder nada;
- **a conta precisa de fator confirmado.** O gate administrativo exige sessão que apresentou o segundo fator (§2), então promover uma conta sem matrícula criaria um administrador que não consegue agir e entregaria capacidade administrativa a uma conta protegida por um segredo só;
- **a instalação não pode ter atribuição ativa — de nenhum papel.** É o que faz do comando um *bootstrap*, e não um caminho para conceder o papel a quem lê o DSN: com atribuição ativa o comando recusa, e é a destituição que devolve a instalação ao estado que o comando exige. O caso de canto (uma instalação com moderador ativo e nenhum administrador) recusa junto e a recuperação é destituir essa atribuição: o comando nunca concede administração por cima de uma atribuição existente. Uma promoção de uma conta cuja atribuição foi revogada revive a linha, mantendo intactos `granted_by` e `granted_at` — o schema declara a origem imutável;
- **a decisão e a escrita são um passo atômico.** A leitura "não há administrador" é feita sob um lock de tabela por transação, então dois comandos simultâneos não criam dois primeiros administradores: um insere, o outro encontra o administrador que o primeiro criou;
- **nada é gravado sem a trilha.** O evento de auditoria commita na mesma transação que a atribuição — uma promoção que a trilha não podia registrar não acontece. As ações são `administration.role_granted` e `administration.role_revoked`, com o motivo em código estável (`administrative_bootstrap`, `administrative_demotion`) e metadados que só admitem identificador, papel e a transição: não existe chave para endereço, sessão, segredo ou frase livre, então a trilha não pode carregar o endereço da pessoa; o **ator** registrado é a própria conta promovida, porque um comando local não tem conta de operador e inventar uma identidade que não agiu seria um registro pior que o honesto.

`arena admin revoke` é a volta, auditada pela mesma trilha. Ele **não** exige endereço verificado nem fator: perder acesso tem de ser sempre possível, e uma destituição que pudesse ser recusada porque a conta perdeu o fator deixaria a instalação com um administrador que ninguém consegue remover. Nenhum dos dois subcomandos funciona sem confirmação: com terminal, o operador digita o endereço de volta; sem terminal, a mudança exige a flag explícita `--yes`, porque silêncio nunca autoriza mudança de privilégio.

O que verifica isso: os testes do caso de uso (cada requisito recusado nomeadamente, replay sem segundo fato, corrida do primeiro administrador), os testes de integração contra PostgreSQL (o lock, o *upsert* que revive sem reescrever a origem, a destituição datada uma única vez, seis comandos simultâneos produzindo **um** administrador), o teste do bridge de auditoria sobre a allowlist da trilha, e o teste de ponta a ponta do `cmd/arena`, que promove pelo comando real e lê a linha e o evento gravados.

## 4. Aplicação web

- HTML inicial usa `html/template` com escaping padrão; HTML arbitrário de usuário é proibido.
- Componentes TypeScript usam DOM seguro e `textContent`; `innerHTML` é proibido para dados dinâmicos.
- Conteúdo do MVP é plaintext. Markdown, se introduzido, exige parser com allowlist e sanitização.
- CSRF token em toda mutação baseada em cookie, além de validação de origem quando aplicável: `Origin`/`Referer` precisam casar com a allowlist ou com o `Host` que atende a página, e uma origem presente sem host (`null`) é recusada como qualquer divergência.
- CSP restritiva; módulos e estilos próprios, sem `unsafe-inline` ou `unsafe-eval` por padrão.
- Headers: HSTS, `X-Content-Type-Options`, `Permissions-Policy` e proteção de framing. `Referrer-Policy` é `same-origin`: nada sai para outra origem e a submissão na mesma origem continua carregando a origem real, que é o que a validação de `Origin` do CSRF lê. `no-referrer` está proibido aqui porque anula essa origem — sob essa política o padrão HTML serializa o `Origin` de um envio de formulário como `null` e **todo** envio de browser era recusado (P18-T07D).
- Uploads não entram no MVP. Quando entrarem, usar object storage, tipo detectado, limites e domínio de entrega separado quando necessário.
- Redirecionamentos e URLs externas passam por validação.

## 5. Cache

- Rotas privadas sempre enviam `Cache-Control: private, no-store`.
- Respostas com cookie ou identidade não podem ser armazenadas no edge.
- Cache público não inclui posição individual, saldo, email ou informação administrativa.
- Regras Cloudflare têm testes para visitante, autenticado e conteúdo removido.
- Cache poisoning e cache deception fazem parte dos testes de segurança.

## 6. Abuso e rate limiting

Camadas complementares:

- Cloudflare WAF e limites de borda;
- Turnstile em cadastro, recuperação, publicação de Arena e risco elevado;
- limites no Go por conta, sessão, ação e sinais de rede;
- constraints e idempotência no PostgreSQL;
- detecção e revisão de padrões de Sybil, farming, brigading e reciprocidade.

IP é sinal imperfeito e dado pessoal potencial. Nunca é prova isolada de abuso. A aplicação só confia em headers de IP recebidos de proxies explicitamente confiáveis.

### Limites na aplicação (P16-T03)

Implementados em `internal/platform/ratelimit`: uma tabela de políticas por ação (`auth.register`, `auth.login`, `auth.password_reset_request`, `auth.password_reset_confirm`, `position.confirm`, `position.change`, `argument.publish`, `report.file`, `checkout.create`, `billing.portal`) com orçamento por endereço de rede e, quando autenticado, também por conta. O orçamento por conta é mais apertado que o por endereço: o endereço é sinal bruto (NAT de operadora coloca milhares de pessoas atrás dele) e a conta é exata. Cada recusa é um problem RFC 9457 com `code: rate_limited` e `Retry-After` em delta-seconds arredondado para cima.

Onde os headers de proxy entram: `internal/platform/clientip` só lê `X-Forwarded-For` quando o par imediato da conexão está num CIDR confiável configurado. Sem proxies confiáveis (o default), todo header de encaminhamento é ignorado e a chave é o endereço da conexão — um cliente que varia o header para parecer mil clientes continua sendo um. Com proxies confiáveis, a cadeia é percorrida da direita para a esquerda pulando endereços confiáveis, e a primeira entrada não confiável é o cliente. `X-Real-IP` e `CF-Connecting-IP` não são consultados: são valores únicos, sem cadeia, e a aplicação não tem como distinguir o que o proxy escreveu do que o cliente escreveu.

Limites conhecidos destes limites, que precisam de decisão antes de escalar horizontalmente:

- **o limitador é por processo e em memória.** Com N instâncias o orçamento efetivo é N vezes a tabela: o edge (Cloudflare) é quem limita entre instâncias hoje, e um store compartilhado (Redis) é o passo necessário para fechar a lacuna;
- **a memória é limitada por eviction.** O mapa guarda no máximo `DefaultCapacity` chaves, descartando as menos recentemente usadas, e chaves ociosas expiram. O custo honesto dessa escolha é que eviction esquece um balde: quem inunda chaves distintas (botnet) pode zerar a contagem das chaves que força para fora. O limite absoluto de memória é a razão de aceitar isso; contra um botnet, quem limita é o edge;
- **endereços em memória são dado pessoal potencial.** Não são persistidos, não entram em log, não entram em métrica, e uma decisão de recusa carrega apenas o tipo de dimensão que recusou, nunca o valor;
- **as políticas são pontos de partida conservadores**, derivados do custo de servir uma requisição aceita (hash Argon2id, email enviado, chamada ao Stripe, atenção humana na fila). Ajuste real depende de tráfego real (fase 28); afrouxar uma linha é decisão registrada, não conveniência local.

### Desafio anti-bot (P16-T04)

Implementado em `internal/platform/turnstile`. O desafio é resolvido no browser pelo widget do provedor e **verificado apenas no servidor**: o browser recebe um token, nunca o segredo. O segredo (`ARENA_TURNSTILE_SECRET_KEY`) existe só no processo, é enviado apenas ao endpoint de verificação do provedor, e `Config.String`/`GoString` o redigem para que nenhuma linha de log o carregue; um teste percorre as respostas das rotas guardadas e reprova se o valor aparecer em corpo ou header de qualquer resposta.

Política, em uma tabela única (`internal/platform/turnstile`, `requirements`):

- `signup` (cadastro), `password_reset` (recuperação) e `arena_publish` (publicação de Arena) exigem desafio **em toda chamada** — são as ações que criam conta, disparam email e criam conteúdo público;
- `login_elevated` exige desafio **sob risco elevado**, que é o sinal de falhas consecutivas de autenticação por endereço de rede (THR-AUTH-02): quem digita a senha errada algumas vezes não vê desafio, um laço de adivinhação vê. O sinal é contado por endereço, nunca por email, para não virar oráculo de existência de conta, e é zerado no primeiro sucesso;
- uma ação não declarada na tabela resolve para **exigir desafio**, não para liberar: o silêncio de uma linha esquecida tem que falhar fechado.

O que é verificado na resposta do provedor, e por quê:

- `success`;
- **hostname**: o token tem que ter sido resolvido para o hostname configurado (`ARENA_TURNSTILE_HOSTNAME`); um token cunhado para outro site não é gastável aqui;
- **action**: o token tem que ter sido cunhado para a ação que o está gastando, então um token do widget de cadastro não vale numa publicação;
- **uso único**: o token é reivindicado atomicamente antes da verificação, então um replay concorrente não é atendido; quando o provedor também recusa (`timeout-or-duplicate`, que ele não separa de expirado), a recusa é o mesmo `challenge_replayed`. A memória de tokens gastos guarda **fingerprint SHA-256**, nunca o token, e é limitada em tamanho e em tempo (a vida útil de um token do provedor);
- **timeout**: a chamada de verificação tem deadline curto e o corpo da resposta é limitado, então um provedor lento não segura a requisição nem faz o processo ler sem limite.

Política de falha, explícita e configurada (`ARENA_TURNSTILE_FAIL_POLICY`), nunca implícita: o default é **fechado** — um desafio que não pode ser verificado recusa a ação, porque o instante em que o provedor está inacessível é exatamente o instante em que um cliente automatizado gostaria de prosseguir. A política **aberta** existe para um operador que prefere manter o cadastro funcionando durante uma indisponibilidade do provedor; ela precisa ser pedida por nome, vale **somente** para a resposta "não conseguimos verificar" e nunca para um token inválido, replay ou configuração errada (segredo rejeitado pelo provedor é implantação quebrada, não indisponibilidade, e é recusada sob as duas políticas). Um token ausente também não é verificável: é recusado antes de qualquer chamada, sob as duas políticas.

**Onde essa configuração entra:** ler o ambiente é responsabilidade do composition root (`internal/platform/config`, a única camada que toca o processo). As jornadas de conta e de participação passaram a ser compostas em `cmd/arena` (P18-T07A e P18-T07B), mas os três nomes acima (`ARENA_TURNSTILE_SECRET_KEY`, `ARENA_TURNSTILE_HOSTNAME`, `ARENA_TURNSTILE_FAIL_POLICY`) continuam fora de `Load`: a superfície montada não apresenta o desafio (`script-src 'self'` recusa o script do provedor), então o verificador da composição é `nil` e uma variável de segredo entraria sem consumidor. O construtor recebe a configuração como parâmetro explícito e as variáveis entram junto com a instância que as usa — o mesmo critério que a P16-T03 registrou para proxies confiáveis, aplicado na P18-T07B a `ARENA_CURSOR_SECRET`, que entrou em `Load` exatamente com a jornada que assina cursores (e para não documentar uma variável que a validação de ambiente rejeitaria como desconhecida).

O que acontece localmente também é explícito: sem segredo configurado em desenvolvimento ou teste, a composição instala um **fake local documentado** que aceita somente os tokens do widget de teste do provedor e não tem cliente HTTP, endpoint nem segredo (estruturalmente incapaz de falar com a rede); sem segredo em produção, a construção falha no boot em vez de rodar desprotegida.

Limites conhecidos, que precisam de decisão antes de escalar ou antes do widget existir no browser:

- **a memória de tokens gastos é por processo.** Com N instâncias, um replay pode ser reivindicado em outra instância; quem impede é o próprio uso único do provedor, que é a segunda linha por construção. Um store compartilhado é o passo necessário para tornar a garantia local global;
- **o widget no browser ainda não existe** (fase 18). Quando ele existir, a política de CSP da P16-T01 (`script-src 'self'`) precisa permitir explicitamente o host do desafio, e o site key público entra no HTML; a secret continua nunca chegando ao browser. Não se antecipou essa permissão agora porque não há consumidor: afrouxar a CSP sem widget seria reduzir a proteção sem uso;
- **o endpoint de verificação é uma dependência externa no caminho de um cadastro.** É o preço de fazer a pergunta ao provedor em vez de confiar no cliente, e é o motivo de a política de falha ser uma decisão registrada.

## 7. Wallet e concorrência

- Ledger append-only; saldo materializado nunca é autoridade isolada.
- Débito e publicação de argumento na mesma transação.
- Consumo de passe e publicação de Arena na mesma transação.
- Lock e constraints impedem double spend.
- Toda entrada externa repetível possui chave de idempotência.
- Ajustes administrativos exigem motivo, ator e trilha de auditoria.

## 8. Stripe

- Verificar assinatura usando corpo bruto e secret correto do ambiente.
- Persistir `stripe_event_id` único antes de aplicar efeito.
- Não creditar por visita à página de sucesso.
- Processamento idempotente e transacional.
- Não registrar payload integral nem dados desnecessários.
- Reembolsos e chargebacks possuem estados explícitos; nunca fazem mutação silenciosa do ledger.

## 9. Segredos e supply chain

- Nenhum segredo no Git, imagem ou frontend.
- Arquivo de ambiente de produção fora do checkout, owner root e modo `0600`, ou solução equivalente.
- GitHub Actions usa permissões mínimas e OIDC quando o fornecedor suportar.
- Actions de terceiros fixadas por commit SHA.
- Dependabot, `govulncheck`, Gitleaks e scanner de imagem no CI.
- SBOM e provenance entram antes da primeira release pública paga.

## 10. Logs e auditoria

- Logs estruturados possuem request ID, classe de evento e identificadores não sensíveis.
- Nunca registrar senha, token, cookie, segredo, texto de denúncia, corpo integral de webhook ou rascunho.
- Ações administrativas e de moderação entram em trilha separada e imutável por regra de aplicação.
- Sentry recebe redaction antes do envio.

## 11. Checklist de release

- [threat model](THREAT_MODEL.md) revisado;
- testes de autorização e CSRF;
- testes de cache público/privado;
- teste de webhook repetido;
- teste de double spend e corrida;
- restauração de backup comprovada;
- secrets scan limpo;
- dependências sem vulnerabilidade crítica conhecida;
- conta admin com MFA, promovida pelo comando local auditado (§3);
- origem e PostgreSQL não expostos;
- contato privado de segurança disponível.

A checklist acima não é uma leitura: `make security-audit` (P20-T04) roda as doze áreas e confere o registro [SECURITY_AUDIT.md](SECURITY_AUDIT.md) contra o [modelo de ameaças](THREAT_MODEL.md) — mesmo conjunto de ameaças, mesma severidade, nenhuma ameaça Crítica respondida apenas com monitoramento, nenhum achado Crítico ou Alto em aberto e nenhum aceite sem dono e data — resolvendo cada caminho de evidência citado contra a árvore. O que ele **não** cobre da checklist está em [SECURITY_AUDIT.md](SECURITY_AUDIT.md) §5: a esteira cobre o resto (`gitleaks` em `source-scans`, `trivy` em `image-scan`, `make image-verify`), e o gate confere que esses jobs existem em vez de presumi-los.

## 12. Reporte de vulnerabilidade

Não publicar vulnerabilidade explorável em issue. Usar o canal privado de segurança do GitHub definido no arquivo `SECURITY.md` da raiz. Detalhes podem ser publicados após correção coordenada.

O canal **oficial do lançamento** e a publicação (ou não) de prazos de resposta são decisão do proprietário ainda **em aberto**: bloqueio `security-channel` de [GOVERNANCE.md](GOVERNANCE.md). Enquanto ele existir, o item “contato privado de segurança disponível” do checklist da §11 não pode ser marcado — o canal privado existe, mas ninguém decidiu que ele é o oficial nem o que ele promete a quem reporta.
