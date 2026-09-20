# Deploy e operação

**Status:** plano inicial; nenhum ambiente foi provisionado por este documento

## 1. Sistema operacional e topologia

- Debian estável ou Ubuntu LTS com atualizações de segurança.
- Docker Engine e Compose plugin.
- Containers iniciais: `caddy`, `app`, `worker` e `postgres`.
- Mesmo artefato imutável para app e worker, com comandos diferentes.
- Volumes persistentes apenas para PostgreSQL, Caddy e dados operacionais necessários.
- Upload público nunca fica no filesystem local da VPS.

Somente HTTP/HTTPS chegam publicamente à origem. Administração usa Tailscale preferencialmente; se SSH público for temporariamente necessário, somente chave, usuário não-root e limite de origem.

## 2. Orçamento inicial da VPS de 16 GB

Valores são limites de investigação, não promessa de capacidade:

- PostgreSQL: aproximadamente 4–6 GB entre buffers e processos.
- Aplicação e worker: limite combinado inicial abaixo de 2 GB.
- Caddy: abaixo de 256 MB em operação comum.
- Sistema, agentes e margem operacional: aproximadamente 2 GB.
- Restante preservado para page cache e picos.

Parâmetros como `shared_buffers`, `work_mem`, conexões e pool serão definidos por benchmark com CPU, NVMe e workload reais. Configuração copiada de outra VPS é proibida sem medição.

## 3. Rede

- PostgreSQL escuta apenas na rede privada do Compose.
- Caddy é o único container com portas públicas.
- Origem aceita tráfego web somente do Cloudflare quando a operação estiver estabilizada.
- Cloudflare usa modo TLS `Full (strict)` até a origem; `Flexible` é proibido.
- O certificado da origem será Cloudflare Origin CA ou ACME por DNS-01. A escolha operacional final deve evitar reabrir a origem apenas para renovação.
- Headers de proxy são confiados apenas a CIDRs configurados.
- IPv4 e IPv6 seguem a mesma política; não deixar origem exposta por uma família esquecida.
- Egress é permitido apenas conforme necessidade e monitorado para jobs sensíveis.

## 4. Ambientes

- `local`: dados sintéticos, compose de desenvolvimento.
- `ci`: banco descartável por execução.
- `staging`: configuração semelhante, sem dados pessoais de produção.
- `production`: dados reais, secrets e volumes separados.

Nunca copiar banco de produção integral para desenvolvimento. Fixtures e dados anonimizados são a alternativa.

## 5. Pipeline

1. PR executa TypeScript estrito, build ESM, testes Go/browser, contrato, migrations, segurança e build da imagem.
2. Merge em `main` produz imagem OCI no GitHub Container Registry.
3. Release promove uma imagem por digest, não recompila na VPS.
4. Backup e verificações pré-deploy são executados.
5. Migrations compatíveis são aplicadas por papel separado.
6. Containers são atualizados e health checks confirmados.
7. Smoke tests exercitam leitura, autenticação e dependências críticas.
8. Falha faz rollback da aplicação; migration destrutiva nunca depende de `down` automático.

### Requisitos de boot do `arena server` (P18-T07A)

O processo compõe as jornadas que serve a partir da configuração, e recusa o boot quando falta o que elas exigem:

- com `ARENA_DATABASE_URL` definida, a jornada de conta do browser é composta e montada no mux da plataforma (dentro das camadas de request id, locale e política de segurança); `ARENA_ASSETS_DIR` passa a ser obrigatória e aponta para um build do `make build-web` (padrão `web/dist`). O boot falha em vez de servir páginas cujos assets não existem;
- sem a DSN o processo serve somente as rotas de health e registra isso no log: um processo que responde 404 em toda página enquanto se declara pronto é pior que uma sonda que diz o que é;
- em produção, a jornada de conta recusa a composição enquanto não existir um adapter de entrega de email (a P15 compôs a fila durável; a entrega pelo provedor ainda é um trabalho pendente), porque um cadastro cujo link de confirmação não sai não é uma jornada. Em desenvolvimento e teste a composição instala o sink local do módulo de identidade e avisa no log que as mensagens são registradas e não entregues.

As rotas que o registro declara e nenhuma superfície monta continuam respondendo como placeholder: o processo declara o contrato inteiro e serve o que foi composto.

Deploy automático em produção só será ativado depois que rollback e restauração tiverem sido testados.

## 6. Estratégia de migrations

Usar expand/contract:

1. adicionar estrutura compatível;
2. publicar código que entende os dois formatos quando necessário;
3. migrar dados em job observável;
4. mudar leituras;
5. remover estrutura antiga em release posterior.

Migrations destrutivas exigem backup, estimativa de lock, janela e ADR quando material.

## 7. Backup

Antes do primeiro usuário pagante:

- WAL contínuo para storage externo compatível com S3;
- base backup diário;
- criptografia em trânsito e repouso;
- credencial de backup com privilégio mínimo;
- retenção documentada;
- teste mensal automatizado de restauração em ambiente isolado;
- alerta para atraso ou falha.

Meta inicial: RPO de até 15 minutos e RTO de até 4 horas. A meta só pode ser publicada como garantia depois de exercícios reais. Backup na mesma VPS não conta como backup.

## 8. Observabilidade

- health endpoints distintos para liveness e readiness;
- logs JSON com rotação no host;
- Sentry para erros de aplicação;
- métricas de CPU, RAM, disco, inode, conexões e latência PostgreSQL;
- alertas para disco, falha de backup, erros 5xx, fila atrasada e webhook falhando;
- PostHog separado de logs operacionais e sem PII.

Adicionar uma stack própria de métricas só quando a solução do provedor deixar lacuna mensurável.

## 9. Cache

Ponto de partida a validar:

- assets com hash: um ano, `immutable`;
- o grafo de módulos ESM também sob o caminho estável (`/assets/pages/auth.js` importa `./submission.js`): o hash não serve para imports relativos, então o caminho estável é servido com revalidação curta (`no-cache`) em vez de imutabilidade; a entrada referenciada pelo HTML é sempre a com hash;
- home e categorias: aproximadamente 10–30 segundos no edge;
- Arena e fragmentos públicos: aproximadamente 30 segundos no edge;
- exportações públicas: TTL curto com ETag;
- rotas privadas, wallet, auth, checkout e admin: `no-store`.

TTL e purge precisam de teste de conteúdo removido. Nenhum número é promessa antes de observar tráfego real.

## 10. Capacidade e escala

Não publicar estimativa de usuários suportados sem teste. k6 mede cenários separados:

- leitura pública com cache frio e quente;
- login e sessão;
- confirmação de posição;
- publicação de argumento com débito;
- webhook repetido;
- Arena viral com maioria de leitores.

Registrar p50, p95, p99, erro, CPU, memória, I/O, conexões, locks e cache hit. A próxima compra de infraestrutura deve responder a um gargalo observado.
