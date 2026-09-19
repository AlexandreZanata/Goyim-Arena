# ADR-014 — MFA por TOTP com implementação nativa do RFC 6238

## Status

Aceito (P16-T05).

## Contexto

Contas operacionais (moderador, admin, security) precisam de um segundo fator
antes do beta público (THR-ADM-01). A P16-T05 pede, literalmente, *avaliar e
registrar*: ou uma dependência TOTP madura, ou uma implementação do RFC bem
testada. A política de dependências (ADR-011, `docs/DEPENDENCIES.md`, AGENTS.md)
exige justificativa técnica documentada, avaliação de alternativas nativas e
registro formal para qualquer dependência nova no backend.

## Decisão

Implementar TOTP (RFC 6238) sobre HOTP (RFC 4226) com a biblioteca padrão do Go,
em `internal/platform/mfa`, e **não** adicionar dependência externa.

A superfície necessária é pequena e inteiramente coberta pela biblioteca padrão:
`crypto/hmac`, `crypto/sha1`, `crypto/sha256`, `crypto/sha512`,
`crypto/subtle` (comparação em tempo constante), `crypto/aes` +
`crypto/cipher` (selagem do segredo em repouso), `crypto/rand` — atrás da porta
`ports.Random`, porque a P02-T02 proíbe leitura direta de efeitos fora de
`clockseed` — para o segredo, o nonce e os backup codes, e `encoding/base32` (o
alfabeto que o RFC 4226 prescreve e que todo autenticador aceita). A correção é
verificável por vetores publicados: o
RFC 4226 Apêndice D e o RFC 6238 Apêndice B são tabelas determinísticas, e o
teste do pacote as executa linha por linha, nos três algoritmos.

O que a implementação precisa decidir além do RFC é justamente o que este
repositório trata como política explícita, não como default de biblioteca:

- **skew limitado**: aceita-se a janela de um passo para trás e um para frente
  (configurável), nunca uma janela larga "para não atrapalhar o usuário";
- **replay de timestep**: o passo aceito é persistido por conta, e um código cujo
  passo já foi usado é recusado mesmo que seja matematicamente válido. Uma
  biblioteca genérica não faz isso, porque não tem onde persistir;
- **segredo selado**: AES-256-GCM com AAD amarrado à conta, para que um segredo
  selado não seja transponível entre contas nem legível por quem lê o banco.

## Alternativas

- **`pquerna/otp`**: madura e popular, mas resolve *só* a geração/validação de
  código. Skew, replay e selagem continuariam sendo código nosso, então a
  dependência cobriria a menor parte do problema e traria uma árvore de
  dependências transitivas para dentro do processo que guarda o fator de
  autenticação;
- **SDK de identidade completo (Auth0, Keycloak, SuperTokens)**: resolve MFA e
  muito mais, ao custo de mover a autoridade de identidade para fora do
  monólito, contradizendo ADR-003 (PostgreSQL como sistema de registro) e a
  arquitetura de monólito modular com autorização server-side (ADR-008);
- **TOTP por e-mail/SMS**: não é segundo fator no sentido do THR-ADM-01 (o mesmo
  canal primário de recuperação), tem custo por mensagem e não funciona offline.
- **`otpauth`-only (sem nosso verifier)**: delegar a validação ao provedor
  externo. Rejeitado: a autorização administrativa não pode depender de um
  terceiro no caminho de cada requisição sensível.

## Consequências

- Nenhuma dependência nova, nenhuma árvore transitiva nova, nenhum binário ou
  imagem a auditar; `govulncheck` continua cobrindo só a biblioteca padrão e o
  conjunto já homologado;
- a superfície criptográfica é pequena e auditável, e cada primitiva usada
  (HMAC, AES-GCM, `subtle`, `crypto/rand` via a porta de entropia, que a
  composição liga a `clockseed.CryptoRandom`) é da biblioteca padrão do Go;
- a responsabilidade de manter o comportamento correto passa a ser do
  repositório: o teste roda os vetores oficiais do RFC nos três algoritmos e
  falha se qualquer parâmetro for alterado, e os casos de skew, replay e
  selagem têm teste próprio;
- se algum dia for necessário suporte a outro esquema (WebAuthn, push), a porta
  do segundo fator é a camada de aplicação do módulo de identidade, e trocar o
  mecanismo não muda o que o resto do sistema vê: uma sessão elevada.
