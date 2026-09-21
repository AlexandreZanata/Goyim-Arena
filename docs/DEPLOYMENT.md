# Deploy e operação

**Status:** plano inicial; nenhum ambiente foi provisionado por este documento

## 1. Sistema operacional e topologia

- Debian estável ou Ubuntu LTS com atualizações de segurança.
- Docker Engine e Compose plugin.
- Containers iniciais: `caddy`, `app`, `worker` e `postgres` (app e worker usam a mesma imagem, com comandos diferentes; P19-T01).
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

### A imagem da aplicação (P19-T01)

O `Dockerfile` na raiz compõe a imagem em três estágios, e cada um existe por um motivo que a sua ausência quebraria:

- **web** — compila TypeScript 7 em ESM nativo com o `tsc` oficial (`npm ci --ignore-scripts` sobre o lockfile e nada além: sem bundler, por política). Node vive aqui e não sai deste estágio;
- **build** — compila o binário Go e roda `assetgen`, que transforma o grafo emitido mais `web/src` nos endereços com hash e no manifest que o servidor relê no boot. Os dois sistemas de build se encontram uma única vez, em `web/generated`, que nenhum dos dois possui;
- **runtime** — o binário, o build de assets e a base. Nada de gerenciador de pacotes, compilador, shell ou Node.

As bases são **fixadas por digest do manifest list**, nunca por tag flutuante, com a tag mantida ao lado do digest (`node:24-bookworm-slim@sha256:...`, `golang:1.27.1-bookworm@sha256:...`, `gcr.io/distroless/static-debian12:nonroot@sha256:...`): a tag diz o que a imagem é, o digest decide qual imagem é. Atualizar uma base é um commit revisável, não um efeito colateral de um `docker build` de amanhã.

O container resultante tem propriedades que o smoke mede, não que a receita promete:

- **não é root** — `USER 65532:65532`, declarado na receita e conferido no `Config` da imagem construída;
- **filesystem somente leitura** — `docker run --read-only` funciona porque o processo não escreve disco nenhum: o sink de email é recusado em produção, as migrations vivem dentro do binário e os logs saem em stdout. Um deploy que precise de rascunho monta um `tmpfs`, e isso é decisão do Compose, não da imagem;
- **sem árvore de fontes** — a imagem carrega o binário e `web/dist`, e o gate reprova qualquer arquivo de fonte (`*.ts`, `*.go`, `web/src/`), cache (`node_modules`, `GOPATH`, cache de apt) ou shell/compilador que apareça no runtime;
- **migrations dentro do binário** — `arena migrate up` roda a partir da própria imagem, contra um banco vazio, sem nada montado de fora; é o que o smoke executa antes de subir o servidor;
- **`ARENA_ADDR=0.0.0.0:8080` e `ARENA_ASSETS_DIR=/web/dist`** vêm definidos na imagem: o default `127.0.0.1` deixaria o processo inalcançável dentro do próprio namespace de rede, e só o Caddy publica porta. Nenhuma dessas duas é segredo — a configuração continua recusando variável `ARENA_*` desconhecida, e um `ARG`/`ENV` com nome de credencial reprova o gate, porque o que entra num build fica gravado numa camada;
- **metadata por build arg** — `VERSION`, `COMMIT` e `BUILD_DATE` chegam por `-ldflags` ao `buildinfo`; nenhum deles é credencial.

O contexto de build é reduzido pelo `.dockerignore`, que exclui `.git`, `.local`, `.env`/`.env.*`, `node_modules`, `web/dist`, `web/generated` e o tooling local — e a receita copia caminhos explícitos, nunca `COPY . .`. O gate exige as duas coisas: uma varredura que encontre o contexto inteiro oferecido ao build é uma varredura que vai encontrar um segredo.

O que verifica isso:

- `make image-build` constrói a imagem (tag em `IMAGE`, padrão `goyim-arena:local`);
- `make image-verify` é o gate: constrói, sobe um PostgreSQL descartável, aplica as migrations **de dentro da imagem**, sobe o container com filesystem somente leitura, prova `/health/live` 200, uma página (`/login` em `text/html`), um endereço com hash lido do manifest **de dentro da imagem** e um 404 para endereço que o manifest nunca declarou, e então entrega receita, contexto, `Config` e filesystem exportado a `tools/imageaudit`;
- `make image-scan` procura vulnerabilidades conhecidas na imagem construída, com `--severity CRITICAL,HIGH --ignore-unfixed --exit-code 1`;

Nenhum dos três entra em `make verify`, pelo mesmo motivo de `test-e2e`: exigem um daemon Docker (e o scan exige um scanner instalado fora do repositório, como `test-load-smoke` exige k6). Ausente do alvo, nunca ausente de gate: sem o scanner o alvo falha explicitamente e nunca retorna sucesso falso.

Limites registrados: a imagem é construída para a arquitetura do host (uma matriz multi-arquitetura é trabalho de deploy, não desta tarefa); `tools/imageaudit` varre o conteúdo em busca das formas conhecidas de credencial (DSN de desenvolvimento, chaves de provedor, chaves privadas) e **não pode** provar a ausência de um segredo desconhecido — é por isso que o `.dockerignore` e a cópia explícita continuam sendo a defesa principal; e o scan depende de um banco de vulnerabilidades atualizado no momento da execução.

### Requisitos de boot do `arena server` (P18-T07A, P18-T07B, P18-T07C)

O processo compõe as jornadas que serve a partir da configuração, e recusa o boot quando falta o que elas exigem:

- com `ARENA_DATABASE_URL` definida, a jornada de conta do browser é composta e montada no mux da plataforma (dentro das camadas de request id, locale e política de segurança); `ARENA_ASSETS_DIR` passa a ser obrigatória e aponta para um build do `make build-web` (padrão `web/dist`). O boot falha em vez de servir páginas cujos assets não existem;
- a jornada de participação da Arena (`/arenas/{slug}` e as quatro transições) entra junto, sobre o mesmo pool: é ela que cobra INK, e a publicação de um argumento debita a carteira na mesma transação em que grava o argumento. Ela exige `ARENA_CURSOR_SECRET` (mínimo de 32 bytes) porque assina os cursores das listas públicas; um cursor assinado com chave efêmera deixaria de resolver depois de um reinício, o que a pessoa vive como uma página que quebrou. Em desenvolvimento e teste, sem a variável a jornada **não** é montada e o log diz exatamente isso; em produção o boot é recusado, porque o Arena é o produto;
- sem a DSN o processo serve somente as rotas de health e registra isso no log: um processo que responde 404 em toda página enquanto se declara pronto é pior que uma sonda que diz o que é;
- em produção, a jornada de conta recusa a composição enquanto não existir um adapter de entrega de email (a P15 compôs a fila durável; a entrega pelo provedor ainda é um trabalho pendente), porque um cadastro cujo link de confirmação não sai não é uma jornada. Em desenvolvimento e teste a composição instala o sink local do módulo de identidade e avisa no log que as mensagens são registradas e não entregues.

- o build referenciado pelas páginas é servido pelo **mesmo processo**, a partir do manifest que ele já lê: a superfície publica exatamente os endereços que o build declarou e nada mais. É conteúdo, não operação: ela não entra no registro de rotas nem no contrato OpenAPI (um CSS não é um endpoint), e a política de cache é a da seção 9 — nome com hash `immutable` por um ano, caminho estável do grafo ESM com revalidação. Um endereço que o manifest não publicou responde 404, não há listagem de diretório e um caminho que tente sair do diretório do build não é filtrado, é irrepresentável: a lista de endereços é montada na composição a partir do manifest, e um manifest que descreva algo fora do build recusa o boot.

- em desenvolvimento e teste, `ARENA_EMAIL_SINK_DIR` faz o sink local de email escrever cada mensagem de identidade em um documento JSON no diretório nomeado, em vez de apenas registrar na memória: é assim que uma jornada dirigida **por outro processo** — o harness de browser de `tools/e2e`, ou uma pessoa completando um cadastro à mão — lê o código de confirmação. A variável é recusada em produção, no `Load` e na composição, porque um diretório de códigos de contas reais não é um mecanismo de entrega;

As rotas que o registro declara e nenhuma superfície monta continuam respondendo como placeholder: o processo declara o contrato inteiro e serve o que foi composto.

O gate de browser (`make test-e2e`, P18-T07) dirige esse mesmo processo: `tools/e2e/harness.sh` provisiona um PostgreSQL descartável (removido em qualquer caminho de saída), um diretório de sink próprio, semeia duas contas confirmadas com INK e uma Arena publicada, sobe o binário e roda as jornadas com o Playwright pinado. `tools/e2e/isolation-check.sh` roda antes e recusa o gate se o runner aparecer no pacote do frontend, no build servido ou no binário. Ele não está em `make verify` porque exige um navegador instalado na máquina.

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
