# Stack tecnológica

**Status:** aprovada para o MVP

**Última revisão:** 2026-09-16

## 1. Resumo

Goyim Arena será uma aplicação API-first com backend Go e frontend baseado somente na plataforma nativa do browser. A implantação inicial continua sendo um monólito modular em uma VPS de 16 GB, mas suas interfaces são desenhadas para permitir clientes e processos independentes.

### Frontend

- **Linguagem:** TypeScript 7.x, patch estável mais recente.
- **Saída:** JavaScript ESM nativo gerado pelo compilador oficial `tsc`.
- **Componentes:** Custom Elements/Web Components e composição de HTML sem framework.
- **Rede:** `fetch`, `Request`, `Response`, `AbortController` e Streams quando necessário.
- **Estado:** estado local de componente e estado do servidor; sem store global.
- **Eventos:** `CustomEvent` tipado e eventos DOM nativos.
- **CSS:** CSS nativo com custom properties, `@layer`, container queries e cascade controlada.
- **HTML:** semântico, acessível e progressivamente aprimorado.
- **Dependências no browser:** zero pacotes de terceiros.
- **Dependência de build do frontend:** somente o pacote oficial `typescript`.
- **Build:** sem Vite, Webpack, Rollup, Babel, Sass, PostCSS ou bundler.

### Backend e dados

- **Aplicação:** Go 1.27.x, patch suportado mais recente.
- **HTTP:** `net/http` e `http.ServeMux` da biblioteca padrão.
- **Templates iniciais:** `html/template` da biblioteca padrão para documento semântico e SEO.
- **Contrato:** OpenAPI 3.1 versionado; tipos gerados são artefatos, não runtime.
- **Arquitetura:** domínio + casos de uso + ports + adapters + composition root.
- **Banco:** PostgreSQL 18.x, patch suportado mais recente.
- **Driver:** pgx v5, isolado no adapter PostgreSQL.
- **SQL tipado:** sqlc, também restrito ao adapter.
- **Migrations:** goose com SQL versionado.
- **Sessões:** tokens opacos em cookie e armazenamento PostgreSQL.
- **Senhas:** Argon2id via `golang.org/x/crypto`.
- **Busca inicial:** full-text search e índices GIN do PostgreSQL.
- **Jobs:** PostgreSQL e worker Go com `FOR UPDATE SKIP LOCKED`.

### Operação

- **Borda:** Cloudflare DNS, CDN, WAF e Turnstile.
- **Proxy e TLS:** Caddy 2.
- **Pagamento:** Stripe Checkout, Billing e webhooks.
- **Email:** Resend.
- **Storage de backup:** armazenamento compatível com S3 (o exercício usa MinIO fixado por digest); guarda o WAL e os base backups selados da P19-T04.
- **Storage futuro:** Cloudflare R2 quando arquivos forem necessários.
- **Analytics:** PostHog, alcançado por HTTP atrás de um port, com eventos **allowlisted** (allowlist em código, revisada) e sem conteúdo privado: só viajam um locale validado e um inteiro limitado.
- **Erros:** Sentry, alcançado por HTTP atrás de um port, isolado por adapter; nenhum payload de requisição viaja com o relatório.
- **Métricas:** registry RED (HTTP) e USE (pool, fila) no processo, exposto em texto Prometheus só no listener administrativo.
- **Logs:** `log/slog` em JSON.
- **Deploy:** Docker Compose em Debian estável ou Ubuntu LTS.
- **CI/CD:** GitHub Actions.
- **Carga:** k6.
- **E2E:** Playwright fora do bundle do frontend, em `tools/e2e` (P18-T07).

## 2. O significado de “sem dependências externas”

O browser não baixa nem executa bibliotecas de terceiros. Isso elimina dependência de frameworks e reduz supply chain, JavaScript, atualizações e risco de abandono.

TypeScript não é executado pelo browser: o compilador oficial transforma `.ts` em `.js`. Portanto, `typescript` é uma dependência de desenvolvimento obrigatória e fixada. Playwright e ferramentas de auditoria podem existir no CI, mas nada delas é entregue ao usuário.

APIs externas de negócio — Stripe, Resend, Cloudflare, Sentry e PostHog — ficam atrás de ports do backend. Elas são substituíveis; nenhuma pode contaminar o domínio.

## 3. Política do frontend

### Componentes

Cada componente possui responsabilidade única, contrato explícito e diretório próprio:

```text
web/src/components/argument-card/
  argument-card.ts
  argument-card.css
  argument-card.test.ts
```

Regras:

- nomes customizados usam prefixo `ga-`, como `<ga-argument-card>`;
- propriedades entram por tipos, atributos ou data inicial serializada com segurança;
- componentes publicam `CustomEvent` documentado, sem conhecer páginas ou serviços concretos;
- efeitos externos passam por interfaces pequenas injetadas;
- nenhum componente importa outro módulo de domínio por caminho interno;
- Light DOM é o padrão para acessibilidade e composição; Shadow DOM apenas quando isolamento real justificar;
- formulários e links funcionam antes do aprimoramento JavaScript sempre que possível;
- conteúdo de usuário usa `textContent`, nunca `innerHTML`.

### CSS

CSS usa camadas estáveis:

```css
@layer reset, tokens, base, layout, components, utilities, overrides;
```

- design tokens são custom properties versionadas;
- componentes não dependem da ordem de importação acidental;
- layout usa Grid, Flexbox e container queries;
- especificidade permanece baixa com `:where()`;
- seletores são locais ao componente, com prefixo `ga-` ou atributo explícito;
- temas usam tokens, não duplicação de folhas;
- estilos inline e `!important` exigem justificativa;
- acessibilidade respeita contraste, teclado, `prefers-reduced-motion` e zoom.

### Módulos e carregamento

- código emitido é ESM nativo;
- páginas carregam apenas o entrypoint necessário;
- módulos são cacheados com nome versionado ou manifest de assets;
- não há client-side router no MVP; navegação usa URLs e histórico nativos;
- o servidor entrega HTML útil e o TypeScript aprimora interação;
- compressão ocorre no Caddy/Cloudflare; minificador não é requisito inicial.

## 4. Política do backend

O backend não é “100% sem acoplamento”, pois todo software possui dependências. A regra verificável é: domínio e casos de uso não importam HTTP, PostgreSQL, Stripe, email, analytics ou frameworks.

- `domain`: entidades, value objects, invariantes e erros; Go puro.
- `application`: casos de uso e ports necessários; orquestra transações.
- `adapters/in/http`: traduz HTTP/OpenAPI para comandos e respostas.
- `adapters/in/jobs`: executa os mesmos casos de uso fora do HTTP.
- `adapters/out/postgres`: pgx, sqlc, queries e locks.
- `adapters/out/stripe`: pagamento e webhooks.
- `adapters/out/email`: Resend.
- `platform/observability`: ports (`ErrorReporter`, `EventSink`) e os adapters de Sentry e PostHog sobre `net/http`, além do registry de métricas (RED/USE) e do allowlist de eventos analytics. Os provedores são chamados por HTTP (`net/http`), sem SDK, e os tipos deles nunca saem do adapter.
- `bootstrap`: composição das implementações; nenhuma regra de negócio.

Interfaces pertencem ao consumidor, não ao adapter. Evitar repositories genéricos; cada port expressa intenção do caso de uso. HTTP, worker, CLI e futuros clientes reutilizam os mesmos casos de uso.

Um gerador interno em Go poderá ler o subconjunto versionado do contrato OpenAPI e emitir `contracts/generated.ts`. Assim o frontend continua tendo somente TypeScript como dependência de build e os tipos mecânicos não são duplicados manualmente.

## 5. Política de versões

- TypeScript: linha 7, patch estável mais recente testado; 7.0 é estável desde julho de 2026.
- Go: linha 1.27, patch mais recente testado; atualmente 1.27.1.
- PostgreSQL: linha 18, patch mais recente testado; atualmente 18.6.
- Builds fixam versões e checksums; produção usa imagens por digest.
- JavaScript emitido tem alvo definido pela política de browsers, não pela versão do compilador.
- Atualização só avança após CI, testes de contrato, browser e rollback.

## 6. Ferramentas de desenvolvimento

- `tsc --noEmit` para tipos e `tsc` para emissão ESM.
- `tools/webaudit` (`make audit-web`, dentro de `make verify`) mede o build entregue contra os orçamentos de [FRONTEND.md](FRONTEND.md) §11 e as regras de dependência: JS comprimido por página pública, CSS inicial, imports externos, bare specifiers, imports não publicados, construtos que a CSP servida recusa e primitivas de rede fora de `web/src/core`.
- APIs nativas de teste para unidades puras; Playwright para comportamento real do browser, no pacote isolado `tools/e2e` (`make test-e2e`), cujas jornadas dirigem o binário real contra um PostgreSQL descartável — nada do runner está em `web/`, no build referenciado pelas páginas ou no binário entregue. O harness acrescenta a tag de build `pseudolocale` ao servidor das jornadas, que é o único jeito de servir o pseudo-locale de layout; o binário entregue não é compilado com ela (provado por `go test ./internal/i18n/...`), e a execução falha de imediato se o servidor das jornadas não servir o locale.
- `tools/i18naudit` (`make audit-i18n`, dentro de `make verify`) lê a árvore entregue em busca de documento que escreve a própria linguagem (ou nenhuma), prosa fora do catálogo e propriedade física de direção em folha de estilo. Só biblioteca padrão, nada é reescrito e nada é entregue ao usuário.
- `tools/imageaudit` (`make image-verify`, `make image-scan`, P19-T01) julga a imagem de produção duas vezes: a receita (`Dockerfile`) e o artefato (o `Config` da imagem e o filesystem que `docker export` escreve). Reprova base sem digest, `ADD`, `COPY . .`, `ARG`/`ENV` com nome de credencial, estágio de runtime sem `USER` não-root, contexto sem `.dockerignore` (ou com cobertura incompleta), e no runtime qualquer compilador, interpretador, shell, cache de pacote, arquivo de ambiente, dado de controle de versão, fonte ou credencial conhecida. É biblioteca padrão, não escreve nada e nada dele vai para a imagem; `make image-verify` compõe o resto (build, PostgreSQL descartável, migrations de dentro da imagem, container somente leitura e smoke de página e asset).
- `tools/composeaudit` (`make compose-verify`, P19-T02) julga a topologia de produção: o documento que `docker compose config --format json` **cria** — e não o arquivo como escrito — mais uma varredura textual do arquivo commitado. Reprova imagem sem digest, serviço que constrói, mais de um serviço publicando porta, porta publicada que não seja o HTTP/HTTPS do ingress, a porta do banco publicada, rede não declarada, rede do banco que não seja `internal`, ingress compartilhando rede com o banco, serviço sem `restart`, sem limites de CPU/memória ou sem `logging.options.max-size` (log sem limite enche o disco onde o banco escreve), serviço de terceiro sem healthcheck (o da imagem da aplicação pode omiti-lo porque a imagem distroless não tem o que executar, mas então exige `stop_grace_period`), o artefato rodando em um só papel ou nos dois com o mesmo comando, papel da aplicação sem rota de saída (só em rede `internal`), variável de produção ausente e a senha do banco num processo que não abre o banco. A varredura textual existe porque, depois de renderizado, todo arquivo de ambiente já foi embutido no documento: o arquivo commitado é o único lugar onde "o valor está escrito aqui" é visível. Nunca imprime valor de ambiente — o documento renderizado carrega as credenciais do operador —, e `make compose-verify` faz o trabalho de Docker (registry descartável para promover por digest, stack, migrations, cadastro pelo ingress, inspeção de portas e limites, restart e recriação sem perda). O resultado da tentativa da entrega é impresso em vez de julgado: a asserção é que a fila foi consumida, e a rota de saída até o provedor só é afirmada quando o detalhe gravado mostra que ele respondeu.
- `go test`, race detector, fuzzing dirigido e benchmarks.
- `golangci-lint` e `govulncheck`.
- `sqlc vet` e migrations em banco descartável.
- Testcontainers-Go para integrações com PostgreSQL real.
- `tools/caddyaudit` (`make caddy-verify`, P19-T03) julga a origem Caddy: o arquivo commitado, lido como statements, mais o comportamento de um Caddy de verdade. Reprova endpoint de admin que não seja loopback (ou ausente, ou desligado), lista de proxies confiáveis ausente, vazia, ilegível, com prefixo que não é CIDR ou com `/0` (confiar em todos apaga a diferença entre o CDN e qualquer um), header de cliente não estreitado a um header de valor único, `strict_sni_host` desligado, listener que também aceita QUIC, listener sem orçamento de `read_header`/`idle`, certificado que não venha de arquivo *secret*, piso de TLS abaixo de 1.2, resposta que nomeia o software no caminho proxiado ou na resposta de erro que o próprio edge escreve, `encode` com algoritmo não declarado, upstream escrito no arquivo em vez de vir do ambiente, `X-Forwarded-For` não reescrito a partir de `{client_ip}`, `CF-Connecting-IP`/`X-Real-IP` do cliente não removidos, upstream sem sonda ativa ou sem tempo de transporte, `Cache-Control` declarado pelo edge no caminho proxied, rota para o endpoint de admin, mais de um site, e qualquer divergência entre a política de segurança que os erros do edge carregam e a que a aplicação entrega — comparada com `securityheaders.Policy(true)`, porque uma segunda cópia de uma política é uma cópia que deriva. O gate exige que o arquivo seja o que `caddy fmt` escreveria, valida com a **imagem que o compose fixa** (o digest é lido do próprio `compose.production.yaml`) e roda o Caddy atrás de um upstream stub em rede própria: confirma que `Cache-Control`/`ETag` da aplicação passam intactos, que a resposta comprimida varia em `Accept-Encoding` e tem validador próprio, que o header forjado não sobrevive enquanto o controle positivo (todos confiáveis) o acredita, que o handshake de outro nome termina sem certificado, que o caminho de admin chega na aplicação como 404 e que uma indisponibilidade responde com a política, `no-store` e sem nome de servidor — o upstream do gate declara um `Server` próprio, porque com um upstream silencioso a asserção sobre vazamento de nome mediria nada. Também entram ali as regras de fixture e o stub, que é `FROM scratch` com um binário estático.
- `tools/backupctl` (`make backup-verify`, P19-T04) é a ferramenta do pipeline de backup e o auditor da parte dele que mora no arquivo commitado. Julga o `compose.production.yaml` como texto — `archive_mode`/`wal_level`/`archive_command`/`archive_timeout`/`wal_keep_size` presentes e coerentes, `archive_timeout` dentro do teto de RPO que o arquivo publica, script, ferramenta e chave montados, chave que não seja variável de ambiente — e recusa o arquivo quando o serviço `db` não pode arquivar. O resto é a metade de comportamento, em `deploy/backup/verify.sh`: sobe um PostgreSQL descartável com os argumentos lidos **do próprio arquivo**, mede o arquivamento em `pg_stat_archiver`, cifra e envia um base backup, destrói o primário, restaura num cluster vazio até um instante escolhido e compara o que voltou (linhas, checksum, registros de migration com suas versões, conjunto de objetos de schema), prova que a linha marcada está no primário e não no objeto que o armazenamento guarda, que outra chave não abre o objeto, que a retenção em seco seleciona só o backup antigo sem apagar nada, que aplicar retenção remove o par de objetos do antigo e mantém o par do restaurável, e que o piso de WAL nomeia o segmento de que o backup mantido precisa — medindo, no fim, o RPO observado e o RTO do exercício. Nada de terceiro: apenas biblioteca padrão, assinatura SigV4 do próprio repositório e `pg_basebackup`/`psql` do PostgreSQL.
- `deploy/deploy.sh` (`make deploy-verify`, P19-T07) é o pipeline de deploy e rollback: promove uma release por digest — recusando tag e digest malformado —, aplica as migrations com a própria imagem da release (`migrate up`, o único caminho que existe), sobe `app` e `worker` sem tocar no banco (`--no-deps`) e só considera a release promovida depois de a prontidão responder 200 na superfície pública, cada caminho de smoke responder 200 e o contêiner que está a correr reportar o digest promovido. Qualquer uma das três a falhar devolve a release anterior, prova-a de novo e diz que não promoveu; o arquivo de estado do operador não avança. `tools/deployaudit` é o exercício: registry descartável, `db` e `caddy` e mais nada, as recusas de preflight (tag, digest malformado, deploy sem sonda, documento que roda outro artefato, banco parado, ausência de `migrate down`), o primeiro deploy migrando e servindo, a rollback trazendo a versão anterior de volta com o schema intacto, e duas releases que não ficam prontas — uma que nunca responde à prontidão, outra que responde e perdeu a página — sem que nenhuma delas seja promovida nem registrada. O *stand-in* de release (`tools/deployaudit/stub`, três modos embutidos na imagem) é só biblioteca padrão e diz o que é; o gate exige daemon Docker, como `image-verify`.
- `tools/migrationaudit` (`make migration-audit`, P20-T03) é a auditoria do ciclo de vida das migrations e o portão que a transforma em recusa. Exige PostgreSQL de verdade — SQLite e dublês são proibidos pelo plano, e um gate que medisse outro motor estaria a responder outra pergunta —, e é por isso que o alvo compõe o Docker: `tools/migrationaudit/verify.sh` sobe a imagem `postgres:18.4` **fixada por digest**, recusa versão diferente de 18 e entrega o DSN à ferramenta, que cria e derruba os próprios bancos (`arena_migaudit_<pid>_*`). O exercício: banco vazio construído pelo `dbmigrate` (a referência), um degrau que aplica uma migration por vez enquanto outra sessão segura `ACCESS SHARE` em todas as tabelas do schema — registrando em que relação e sob qual lock a migration esperou, que é a estimativa de janela que a [DEPLOYMENT.md](DEPLOYMENT.md) §6 exige —, um snapshot copiado **depois de cada versão** e rolado para o head (o resultado tem de ser o mesmo banco, sem perder linha nem coluna), e uma migration injetada que cria uma tabela, insere uma linha e divide por zero numa base que ainda tinha migrations pendentes, seguida da recuperação. Depois julga o catálogo que a história produziu: correspondência entre as tabelas que as metades *Up* declaram e as que existem; dono de cada objeto; atributos da role de runtime; `USAGE` sem `CREATE` no schema; nenhum privilégio para `PUBLIC`; a tabela de versão somente leitura; cada tabela classificada em mutável, append-only, somente leitura ou apagável, com as exceções em ledger e o ledger conferido **nos dois sentidos** (uma classificação que deixou de casar com o schema reprova); sequences com `USAGE`/`SELECT` exactamente nas tabelas em que o runtime insere e nunca `UPDATE`; índices válidos e sem duplicata (índice só de expressão não entra na comparação de colunas, que não o sabe julgar); chaves estrangeiras locais ao schema, cascatas declaradas, e a cobertura por índice das chaves — esta última é a única **advertência** do portão, com o trabalho seguinte nomeado, porque uma chave sem índice custa uma varredura no caminho de exclusão do pai e não torna uma release insegura por si; e o dataset que atravessou todas as versões. Reprova ainda a leitura de um `DROP` que só existe em comentário. O que ele imprime é a evidência versionada (`docs/MIGRATION_AUDIT.md`), com tempos, dataset e o detalhe de cada regra; o `-report` é o único arquivo que escreve.
- `tools/i18nrelease` (`make i18n-audit`, P20-T09) é a auditoria final de internacionalização e o portão que a transforma em recusa. Ele mede a árvore duas vezes de propósito: paridade de chaves, igualdade de placeholders, mensagens vazias e códigos estáveis de Problem Details são recalculados a partir de `locales/` **sem chamar o gerador** — um portão que pergunta ao produtor se o produto está certo é um portão que concorda consigo mesmo. As 200 mensagens de cada locale são casadas com o código entregue por varredura de literais, com hastes terminadas em ponto contadas como chave alcançável (`"arenas.document.status."+status`), e a cobertura é 100%: zero órfã. Ele recusa chave inalcançável ou literal com forma de chave que o registro não nomeie, mensagem vazia, placeholder divergente, código de erro que exista em um idioma e falte no outro, snapshot de email ausente para algum par template×locale, chave de plural contada a mais, `hreflang` sem página equivalente, `toLocaleString` na árvore entregue (§6 não tem exceção), resposta variada pelo header `Accept-Language` bruto que o registro não nomeie com dono (§4), pseudo-locale que vaze para o catálogo de produção, idioma de jornada que o produto não embarca, idioma de conteúdo do harness que divirja do que o seed declara, achado Crítico ou Alto em aberto, aceite sem dono ou sem trabalho seguinte, revisão humana marcada como feita sem revisor e sem data (ou pendente sem quem decide e sem o que falta), menos de cinco limitações e qualquer endereço fora das zonas do RFC 2606. O registro é [I18N_AUDIT.md](I18N_AUDIT.md): prosa para o leitor, bloco de máquina para a ferramenta — e o bloco é do **autor**, não da ferramenta: `-run` executa as áreas e grava só as execuções, `-check` compara as afirmações com a árvore. Ele é biblioteca padrão mais as declarações de `internal/notifications/domain` (para medir os pares template×locale de email), roda dentro de `make verify` e nunca reescreve o documento.
- `tools/releaseverify` (`make release-verify`, P20-T07) é a verificação final reproduzível e o portão que a transforma em recusa. O exercício (`verify.sh`) cria um checkout limpo do commit com `git worktree`, **declara a sobreposição** dos arquivos da própria tarefa que copia por cima (a ferramenta da verificação não pode estar dentro do commit que verifica: ela é o instrumento, e entra no mesmo commit que o documento que produz) com o digest de cada um, instala as dependências só pelos lockfiles (`go mod download`, `npm ci` nos dois pacotes, `sqlc` na versão fixada em um `GOBIN` privado), usa o PostgreSQL que os harnesses procuram (127.0.0.1:54329 — a porta do `compose.yaml` e do serviço do CI — subindo a imagem fixada por digest quando nada responde ali, e recusando o run quando o servidor carrega bancos `arena_test_*` de uma execução interrompida), roda `make verify`, constrói a imagem e a exercita com `make image-verify`, e confere árvore, `git fsck` e o portão de release. Cada comando roda **duas vezes** e um vermelho **aborta antes de escrever qualquer coisa**. O que ele mediu vira `docs/RELEASE_CHECKLIST.md`, renderizado do próprio JSON pelo subcomando `render` (a prosa sai das medições, nunca da mão de quem escreve); o subcomando `check` julga o documento entregue e recusa comando que rodou uma vez, duração ausente ou impossível, execução vermelha, commit que não existe no repositório, máquina sem o PostgreSQL 18, lockfile ausente ou contornado por `npm install`, sobreposição não declarada, fora do escopo (código de produto) ou com digest que não bate com a árvore, imagem sem identificação, digest ou smoke, portão de release recusando sem nomear a decisão aberta, limitação vazia ou em número menor que cinco, árvore que terminou guardando mais do que o documento e os arquivos declarados, e endereço de domínio não reservado em qualquer parte do texto. É biblioteca padrão mais `git`; nunca escreve fora do documento e **não** entra em `make verify`, porque roda o próprio `verify` duas vezes.
- `tools/handoffaudit` (`make handoff-check` e `make handoff-walkthrough`, P20-T08) é o portão do handoff e a caminhada que o torna falsificável. A primeira metade lê o [README](../README.md) e recusa toda afirmação que a árvore contradiz, nos dois sentidos: cada `make <alvo>` que a página manda rodar tem de existir no `Makefile`, cada `ARENA_*` tem de estar no `.env.example` (uma família declarada com curinga vale quando o modelo tem um membro dela), cada caminho citado tem de existir, estar num commit ou ser ignorado pela árvore — `web/dist` é saída de build, `.env` é configuração local, e `docs/RELEASE_CHECKLIST.md` é um artefato versionado que o próprio run que o regenera remove do seu checkout —, cada `arena <subcomando>` tem de estar no próprio texto de ajuda do binário, e as seis seções que a fase nomeia têm de estar lá, com os documentos de evidência citados ([RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md), [GOVERNANCE.md](GOVERNANCE.md), [CI.md](CI.md)) e nenhum endereço fora das zonas do RFC 2606 nem valor com forma de credencial. A segunda metade não julga a página: ela **segue** a página. Cria um `git worktree` desacoplado no commit, põe por cima as mudanças da árvore de trabalho e as fontes da própria ferramenta (declaradas, com o digest de cada uma — o instrumento não pode estar dentro do commit que verifica, e o `Makefile` que declara os alvos citados é uma dessas mudanças), roda a jornada de comandos que o bloco `quickstart` do README manda colar dentro de um checkout limpo, e depois sobe o servidor exatamente como o bloco `serve` manda — com as exportações da própria página, e com o ambiente `ARENA_*` do operador **removido** — para sondar as superfícies que ela nomeia. Uma página que não funciona fica vermelha em vez de virar um parágrafo em que alguém acredita, e foi assim que ela achou dois defeitos reais: a regra dos caminhos perguntava ao git por `web/dist` numa árvore onde o diretório ainda não existe (um padrão terminado em barra só casa com diretório, então a resposta era "não ignorado") e o `git status --porcelain` era aparado como se fosse uma frase, perdendo a primeira coluna do primeiro arquivo.
- `tools/privacyaudit` (`make privacy-audit`, P20-T06) é a revisão de privacidade e moderação e o portão que a transforma em recusa. Lê o registro versionado (`docs/PRIVACY_AUDIT.md`: prosa para o leitor, bloco JSON para a ferramenta) e recusa bloco ausente, campo que ele não sabe julgar, documento sem prosa, versão de esquema desconhecida, data fora de `YYYY-MM-DD`, conta sintética que não seja exatamente duas, com papéis repetidos, endereços iguais ou domínio fora das zonas reservadas (RFC 2606) — e recusa **qualquer** endereço de domínio não reservado em qualquer parte do documento, que é a validação da fase ("o relatório não contém PII real") feita de forma mecânica. Confere o registro **contra o código que ele descreve**, nos dois sentidos: as chaves JSON que cada superfície de exportação pode emitir contra a allowlist declarada (chave que o código emite e o registro omite reprova, e chave declarada que o código não emite também), a forma de cada chave contra as categorias que a política diz que nunca aparecem — com casamento por token, para que `ip_address` seja recusado e `participants_total` não; a tabela de retenção contra o cronograma que o job aplica, linha a linha (classe, ação, janela, caráter indefinido, código de razão, nas duas direções); o limiar de baixa contagem contra o dos agregados, exercitado nas duas bordas; e o vocabulário de analytics contra a allowlist do despachante, evento a evento e propriedade a propriedade. Resolve contra a árvore cada caminho citado por área e por achado, exige as **sete** áreas da fase com veredito do vocabulário e uma execução, exige que a evidência de mistura de dados exista e **nomeie as duas contas** ("usamos duas contas" é frase; um teste que segura as duas é evidência), recusa achado Crítico ou Alto em aberto e aceite sem dono e data, e recusa execução vermelha ou que não tenha casado nenhum teste. O modo `-check` julga o registro sem executar nada. É biblioteca padrão; nunca escreve e **não** entra em `make verify`, porque é trabalho de release como o `security-audit`.
- `tools/secaudit` (`make security-audit`, P20-T04) é a auditoria de segurança e o portão que a transforma em recusa. Lê o registro versionado (`docs/SECURITY_AUDIT.md`: prosa para o leitor, bloco JSON para a ferramenta) e recusa bloco ausente, JSON inválido, campo que ele não sabe julgar, data fora de `YYYY-MM-DD`, identificador fora da convenção, entrada repetida e frase ausente. Confere o registro **contra o modelo de ameaças**: o conjunto de ameaças tem de ser exatamente o do `docs/THREAT_MODEL.md` (uma ameaça não auditada ou inventada reprova), a severidade tem de ser a do modelo (rebaixar é o que esta regra existe para impedir), nenhuma ameaça **Crítica** pode ser respondida só com monitoramento (o gate §1 do próprio modelo) e nenhuma pode ficar em aberto. Confere a matriz de testes nomeia as mesmas ameaças. Exige as **doze** áreas da fase, cada uma com uma execução **ou** um job de CI nomeado — e o job tem de existir no workflow, porque "a esteira cobre" é verificação, não frase. Resolve cada caminho de evidência citado contra a árvore: referência que não existe é afirmação sobre nada. Recusa achado Crítico ou Alto em aberto (a validação mínima da fase) e aceite de Média ou acima sem dono e data. E varre a árvore em busca de segredo estrutural — `.env` rastreado, `.env` fora do `.gitignore`, chave privada em tipo de arquivo que nunca a guarda como texto. Por último roda as doze execuções declaradas e transforma cada uma vermelha em violação nomeada, com a saída do próprio comando. O modo `-check` julga o registro sem executar nada. É biblioteca padrão mais `git ls-files`; nunca escreve e **não** entra em `make verify`, porque as execuções incluem `make vuln` e suítes com `-race` e um gate de merge não é um release.
- `tools/drillaudit` (`make disaster-drill`, P20-T05) é o exercício de desastre e carga da release, e a metade dele que transforma o que foi medido em recusa. O exercício (`tools/drillaudit/verify.sh`) usa o caminho que um incidente usa, não um caminho paralelo: os argumentos do servidor vêm do `compose.production.yaml` (via `backupctl check-compose` e `print-compose-command`), os scripts da operação fazem o backup (`deploy/backup/base-backup.sh`) e a recuperação ponto-no-tempo (`deploy/backup/restore.sh`), o dataset é criado pelos casos de uso do produto (`tools/e2e/seed`: cadastro, confirmação e crédito de INK pelo caso da carteira), a aplicação sobe contra o cluster restaurado e as jornadas são as versionadas (`tools/e2e/harness.sh`, `tests/load/smoke.js`). O que ele mede: leitura financeira **antes** da perda e **depois** da recuperação (`drillaudit snapshot`, `verify`), comparadas carteira a carteira, digest a digest, agregado a agregado, mais as invariantes do schema (saldo nunca negativo, projeção derivada do ledger, um bucket por operação por operação); o que o alvo não pode trazer (linhas escritas depois dele: um job e uma conta inteira com INK); RPO observado contra o `archive_timeout` que o servidor declara e contra a meta publicada; RTO até o servidor aceitar escrita e até a aplicação responder; os limiares que o workload versionado declara, com o valor medido por `k6` e o veredito de cruzamento do próprio `k6`; e os dois provedores indisponíveis — o email com o transporte do worker apontado para uma porta fechada (tentativa registrada, job retido, nenhuma linha financeira, e depois entregue com um provedor que responde: `tools/drillaudit/vendorsim`, um stand-in que fala o protocolo de proxy do worker e apresenta um certificado próprio como raiz de confiança, para que a verificação de TLS do adapter continue valendo), e o pagamento pela superfície que o contrato declara. Os fatos viram `docs/DISASTER_DRILL.md` — prosa para o leitor, bloco JSON para a ferramenta —, e `drillaudit check` recusa o documento quando ele diz menos do que mediu: versão, data, commit e hardware ausentes; baseline sem carteira ou sem linha de ledger (uma comparação de nada); linhas que voltaram do lado errado do alvo; RPO acima do teto do servidor ou da meta; RTO acima da meta; limiar sem valor medido, valor que não é número, métrica que o workload não declara ou bound cruzado; provedor de email exercitado numa direção só; superfície de pagamento que não está composta sem dono e trabalho seguinte nomeados; jornada não executada ou smoke que não respondeu o que pediu; e a prosa que perdeu um número que o bloco carrega. Nunca escreve fora do documento que lhe é dado, e não entra em `make verify` porque exige daemon Docker, `k6`, navegador e uma build do frontend.
- `tools/testenv` (`make testenv-verify`, P22-T01) é o orquestrador do ambiente descartável e a verificação ao vivo dele. Um comando (`up`, `down`, `status`, `run -- <cmd>`, `isolated`, e os subcomandos que só rodam dentro da rede: `providers`, `forward`, `probe`) provisiona PostgreSQL 18 **fixado no patch** (`postgres:18.4`, o mesmo do `compose.yaml`), as migrations da própria imagem, a aplicação e o worker entregues sobre a base de runtime **fixada por digest** (o terceiro estágio do `Dockerfile`), e a superfície de providers fake — com healthcheck de banco, prontidão medida por HTTP e um namespace por execução (nome, rede, portas, diretórios e segredo de cursor derivados dele, com o registro em arquivo versionado por ele). Ele recusa, com diagnóstico: namespace fora da convenção ou já registrado (construir o mesmo nome duas vezes colidiria rede e portas), porta já ocupada — nomeando a porta, o `lsof` e o `docker ps` que a mostram —, sem daemon, sem build do frontend (a aplicação compõe as páginas a partir dela), rede que não esteja `internal`, e credencial com forma de credencial entre as variáveis que um contêiner recebe. **A resposta de rede não é um detalhe de implementação**: o Docker não publica porta de contêiner em rede `internal`, então a publicação e a rede hermética não caberiam no mesmo contêiner — o ambiente põe os quatro serviços do produto só na rede interna e um **fôro** (`forward`, um encaminhador de alvos fixos que é o próprio binário da ferramenta) dual-homed que publica as três portas em `127.0.0.1`. Só os nomes que o carregador de configuração aceita vão para os serviços, e os nomes próprios do ambiente ficam fora do namespace `ARENA_*` — o produto recusa variável `ARENA_*` desconhecida para pegar erro de digitação, então um nome inventado impediria o serviço de subir, e é isso que a verificação ao vivo confirma. O teardown acontece em todo caminho (comando verde, vermelho, interrupção, `up` que falhou), inclusive fora do contexto cancelado. Os testes do pacote seguram as decisões contra um daemon gravado — ordem de subida, o que cada contêiner recebe, o que é publicado, o que é recusado, o que é derrubado — porque decisão tem de ser demonstrável sem daemon; `tools/testenv/verify.sh` mede as quatro validações da tarefa na máquina, com controle positivo do probe (`docker inspect` de cada serviço, endereço público inalcançável de dentro, e o mesmo probe alcançando a aplicação ali dentro). É biblioteca padrão, e não entra em `make verify` porque exige daemon Docker e uma build do frontend.
- `tools/ciaudit` (`make audit-ci`, dentro de `make verify`, P19-T08) lê os workflows entregues e o `Makefile` como eles são e recusa o CI que não verifica o que a fase exige: gate que nenhum job alcança, alvo invocado que o Makefile não declara, ação presa por tag em vez de commit, token com escrita, passo que engole a própria falha (`|| true`, `continue-on-error`, `if: always()`), job que abre PostgreSQL e não declara o serviço ou aponta para porta que ele não publica, rascunho de PR que deixa de ser adiado e passa a ser aprovado (job sem a condição de rascunho, gatilho sem `ready_for_review`, filtro de caminho), `pull_request_target`/`workflow_run`, segredo além do `GITHUB_TOKEN` da execução, e job sem teto de tempo ou acima do orçamento do pipeline. O scan da imagem é o único gate aceito por ação em vez do alvo, e só quando a ação examina a imagem que aquela execução constrói com os mesmos parâmetros que `make image-scan` declara. A tabela de gates, os dez jobs e o orçamento estão em [CI.md](CI.md); é biblioteca padrão, não escreve nada e roda dentro de `make verify`.
- k6 para capacidade e regressão.
- Gitleaks e Dependabot para supply chain.

## 7. O que não entra no frontend

- React, Vue, Angular, Svelte ou outro framework;
- HTMX, Alpine.js, jQuery ou biblioteca de componentes;
- templ ou JSX;
- Tailwind, Bootstrap, Sass ou CSS-in-JS;
- Redux, Zustand ou store semelhante;
- Vite, Webpack, Rollup, Babel ou bundler;
- dependência carregada de CDN;
- polyfill sem browser-alvo e necessidade documentados.

## 8. O que não entra na infraestrutura inicial

- Supabase;
- Redis ou Valkey;
- Kubernetes;
- microserviços;
- RabbitMQ ou Kafka;
- Elasticsearch, Meilisearch ou Typesense;
- blockchain ou banco vetorial;
- IA no fluxo central;
- uploads no disco local da VPS.

## 9. Regra de expansão

> Nenhuma nova biblioteca ou infraestrutura entra sem problema medido, alternativa nativa avaliada, custo operacional e plano de remoção.

Escala é obtida primeiro com cache, queries corretas, processos stateless, índices, particionamento criterioso e capacidade medida — não com quantidade de tecnologias.

## 10. Referências oficiais

- [TypeScript 7.0](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/)
- [Web Components — MDN](https://developer.mozilla.org/docs/Web/API/Web_components)
- [CSS — MDN](https://developer.mozilla.org/docs/Web/CSS)
- [Histórico de releases do Go](https://go.dev/doc/devel/release)
- [Release notes do PostgreSQL](https://www.postgresql.org/docs/release/)
- [Documentação do Caddy](https://caddyserver.com/docs/)
- [Cache do Cloudflare](https://developers.cloudflare.com/cache/)
