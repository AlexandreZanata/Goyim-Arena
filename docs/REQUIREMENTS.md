# Matriz de requisitos do MVP (Requirements Traceability Matrix)

**Status:** padrão obrigatório

**Documentos de referência:** [MVP.md](MVP.md) · [BUSINESS_RULES.md](BUSINESS_RULES.md) · [ARCHITECTURE.md](ARCHITECTURE.md) · [THREAT_MODEL.md](THREAT_MODEL.md)

**Última revisão:** 2026-09-16

---

## 1. Visão geral e metodologia

Este documento estabelece a rastreabilidade completa e bidirecional de todos os requisitos funcionais e invariantes definidos no escopo do MVP do Goyim Arena ([docs/MVP.md](MVP.md)) e em suas regras de negócio ([docs/BUSINESS_RULES.md](BUSINESS_RULES.md)).

### Regras de governança
1. **Identificador único:** todo requisito funcional ou invariante possui um identificador unívoco no formato `REQ-<CATEGORIA>-<NUM>`.
2. **Critério de inclusão total:** nenhum item classificado como incluído no MVP em `docs/MVP.md` ou `docs/BUSINESS_RULES.md` pode ficar sem requisito correspondente.
3. **Mapeamento quádruplo:** cada requisito mapeia formalmente:
   - Documento e seção de origem;
   - Módulo de negócio proprietário (Ports and Adapters);
   - Microfase de entrega no plano mestre de engenharia;
   - Estratégia de teste automatizado futuro para validação objetiva de aceite.

---

## 2. Matriz de requisitos funcionais por módulo

### 2.1 Identidade e autenticação (`identity`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-AUTH-01** | Cadastro de conta com email e senha forte (Argon2id) e login com emissão de token opaco em cookie seguro (`HttpOnly`, `Secure`, `SameSite=Lax`). | MVP §3 (Conta); BR §1 | `identity` | `04-identity-auth` | Teste de integração de registro/login; teste de hashing Argon2id; validação de atributos de cookie. |
| **REQ-AUTH-02** | Verificação de email via token criptográfico de uso único com validade limitada; apenas contas verificadas participam ativamente. | MVP §3 (Conta); BR §1, §3.1, §7 | `identity` | `04-identity-auth` | Teste de envio de email transacional; teste de confirmação com token; teste de rejeição de token expirado ou reutilizado. |
| **REQ-AUTH-03** | Gerenciamento de sessões ativas com rotação compulsória de token após login/elevação e encerramento voluntário (logout) de sessão atual e de outras sessões. | MVP §3 (Conta); BR §1 | `identity` | `04-identity-auth` | Teste de rotação pós-autenticação; teste de invalidação imediata de token antigo após logout. |
| **REQ-AUTH-04** | Recuperação de conta via link seguro com token de uso único, com respostas que não revelam se o email existe na base (prevenção de enumeração). | MVP §3 (Conta); BR §1 | `identity` | `04-identity-auth` | Teste de Problem Details uniforme para emails existentes e inexistentes; teste de redefinição de senha com sucesso e uso único. |

### 2.2 Perfis, preferências e privacidade (`profiles` & `transparency`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-PROF-01** | Definição e alteração de username público único, validado sintaticamente, sem caracteres de controle ou termos reservados. | MVP §3 (Conta); BR §1 | `profiles` | `05-profiles-privacy` | Teste de validação de formato de username; teste de colisão/unicidade (concorrência de reserva). |
| **REQ-PROF-02** | Configuração de locale e idioma de preferência do usuário (suporte inicial a `pt-BR` e `en-US`). | MVP §3 (Conta, Idiomas) | `profiles` | `05-profiles-privacy` | Teste unitário de preferências de usuário; validação de headers `Accept-Language` e persistência. |
| **REQ-PROF-03** | Exibição de página de perfil público contendo estritamente métricas factuais (argumentos, respostas, fontes, pessoas influenciadas únicas por arena). | MVP §3 (Conta); BR §6 | `profiles` | `05-profiles-privacy` | Teste de contrato da rota de perfil público; verificação de ausência de score universal ou ranking secreto. |
| **REQ-PRIV-01** | Exportação de dados pessoais do titular em formato legível por máquina (JSON) e solicitação de exclusão/anonimização de conta preservando integridade histórica do debate. | MVP §3 (Conta); BR §10 | `transparency` / `profiles` | `14-transparency-privacy` | Teste de integração de exportação de dados; teste de exclusão garantindo remoção de PII e preservação de autoria anonimizada. |

### 2.3 Descoberta e feeds (`arenas`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-DISC-01** | Feed público de arenas segregado por idioma com suporte a paginação keyset ordenada por data de publicação. | MVP §3 (Descoberta, Idiomas) | `arenas` | `08-arenas` | Teste de integração do feed de arenas; validação de paginação estável sem offset profundo. |
| **REQ-DISC-02** | Página pública de arena indexável e semântica com metadados estruturados, afirmação, contexto e listagem de argumentos. | MVP §3 (Descoberta); BR §2 | `arenas` | `08-arenas` | Teste de renderização HTML de arena e metadados; verificação de compatibilidade SEO sem JavaScript. |
| **REQ-DISC-03** | Filtros de listagem de arenas por categoria e por status de ciclo de vida (`published`, `closed`). | MVP §3 (Descoberta); BR §2.2 | `arenas` | `08-arenas` | Teste de consulta com múltiplos filtros combinados (categoria + status + idioma). |

### 2.4 Ciclo de vida da Arena (`arenas` & `entitlements`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-ARN-01** | Criação e edição de rascunhos (`draft`) de arena visíveis exclusivamente ao criador, sem consumir Arena Pass. | MVP §3 (Arena); BR §2.2, §9 | `arenas` | `08-arenas` | Teste de autorização de rascunho (apenas criador acessa); teste de saldo de passe inalterado na criação de rascunho. |
| **REQ-ARN-02** | Publicação de arena com consumo atômico de exatamente 1 Arena Pass; transição atômica de `draft` para `published`. | MVP §3 (Arena); BR §2.2, §9 | `arenas` / `entitlements` | `08-arenas` | Teste transacional de publicação: debita passe e altera status em transação única; rollback se saldo for insuficiente. |
| **REQ-ARN-03** | Definição de afirmação proposicional clara, contexto opcional, categoria, idioma e encerramento programado opcional. | MVP §3 (Arena); BR §2, §2.1 | `arenas` | `08-arenas` | Validação de invariantes de afirmação (não vazia, proposição afirmativa, categoria válida). |
| **REQ-ARN-04** | Máquina de estados formal de arena com transições controladas: `draft` → `published` → `closed` / `restricted` / `removed`. | MVP §3 (Arena); BR §2.2 | `arenas` | `08-arenas` | Teste de máquina de estados; rejeição de transições inválidas (ex.: `removed` para `published`). |
| **REQ-ARN-05** | Imutabilidade absoluta da afirmação principal e de seu idioma após a publicação; proibição de edição pós-publicação no MVP. | MVP §3 (Arena); BR §2.3 | `arenas` | `08-arenas` | Teste de rejeição de mutação de afirmação ou idioma em arena com status `published`. |
| **REQ-ARN-06** | Identificação perene da arena por identificador estável imutável (UUID) complementado por slug amigável para URLs. | BR §2.3 | `arenas` | `08-arenas` | Teste de resolução de arena tanto por UUID quanto por slug; persistência de histórico de redirecionamento se aplicável. |
| **REQ-ARN-07** | Exportação pública versionada dos dados e debates da arena em formato estruturado padronizado. | MVP §3 (Arena); BR §2.3 | `transparency` / `arenas` | `14-transparency-privacy` | Teste de geração e validação de schema do artefato exportado da arena. |

### 2.5 Posições e agregação (`positions`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-POS-01** | Escolha local e volátil de visitante para liberar visualização de agregados na sessão corrente, sem contabilização oficial no banco. | MVP §3 (Posição); BR §3.1 | `positions` | `09-positions` | Teste de contrato validando que escolha de visitante não gera inserção na tabela de posições oficiais. |
| **REQ-POS-02** | Registro e confirmação autenticada de posição inicial oficial entre os valores fixos: `agree`, `disagree` e `undecided`. | MVP §3 (Posição); BR §3 | `positions` | `09-positions` | Teste de persistência de posição inicial para usuário autenticado com email verificado. |
| **REQ-POS-03** | Unicidade e imutabilidade da posição inicial confirmada: cada participante possui no máximo uma posição inicial por arena, que nunca é reescrita. | MVP §3 (Posição); BR §3.2, §11 (Inv 3) | `positions` | `09-positions` | Teste de violação de constraint de banco e rejeição de segunda submissão de posição inicial. |
| **REQ-POS-04** | Posição individual e histórico de mudanças de posição são estritamente privados por padrão; o público visualiza apenas agregados. | MVP §3 (Posição); BR §3.2 | `positions` | `09-positions` | Teste de segurança/autorização: endpoint público não expõe a posição individual de usuários específicos. |
| **REQ-POS-05** | Registro imutável de mudança de posição registrando posição anterior, nova posição e timestamp em trilha histórica append-only. | MVP §3 (Posição); BR §3.3 | `positions` | `09-positions` | Teste de registro de mudança de posição; conferência do encadeamento histórico da posição atual. |
| **REQ-POS-06** | Bloqueio de mudança de posição para o mesmo valor atualmente mantido pelo participante. | BR §3.3 | `positions` | `09-positions` | Teste unitário de domínio rejeitando transição onde `nova_posição == posição_atual`. |
| **REQ-POS-07** | Arenas com status `closed` não aceitam registro de novas posições nem mudanças de posições existentes. | BR §2.2, §3.3 | `positions` | `09-positions` | Teste de caso de uso tentando submeter posição em arena fechada (esperando erro de pré-condição / RFC 9457). |
| **REQ-POS-08** | Exibição de agregados oficiais calculados exclusivamente a partir de contas elegíveis, contendo total de participantes e aviso metodológico de não representatividade. | MVP §3 (Posição); BR §3.1, §7 | `positions` | `09-positions` | Teste de cálculo de agregados oficiais excluindo contas não verificadas e contas suspensas; conferência de metadados. |
| **REQ-POS-09** | Bloqueio server-side de visualização da distribuição agregada de posições antes que o usuário registre sua própria escolha ou posição inicial. | MVP §3 (Posição); BR §3.2; TM THR-PERS-02 | `positions` | `09-positions` | Teste de contrato de rota de arena confirmando mascaramento de agregados para participantes não posicionados. |

### 2.6 Argumentos e respostas (`arguments` & `wallet`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-ARG-01** | Publicação de argumento associado a uma arena com declaração explícita de relação com a afirmação (`a favor`, `contra`, `contextual`). | MVP §3 (Argumentação); BR §4 | `arguments` | `10-arguments` | Teste de persistência de argumento com relação válida; validação de integridade referencial com arena. |
| **REQ-ARG-02** | Publicação de respostas diretamente a outro argumento da mesma arena em estrutura lógica encadeada de 1 nível recursivo. | MVP §3 (Argumentação); BR §4 | `arguments` | `10-arguments` | Teste de submissão de resposta referenciando argumento pai válido; validação de rejeição de cross-arena reply. |
| **REQ-ARG-03** | Adição de fontes estruturadas por URL válida e descrição curta, associadas ao argumento sem custo de INK, com validação de esquema de protocolo. | MVP §3 (Argumentação); BR §4.2, §8 | `arguments` | `10-arguments` | Teste de validação sintática de fontes (rejeição de URLs inválidas ou protocolos perigosos como `javascript:`). |
| **REQ-ARG-04** | Limite estrito de no máximo 3.000 grapheme clusters de texto por argumento, computados com biblioteca precisa de segmentação Unicode. | MVP §3 (Argumentação); BR §4 | `arguments` | `10-arguments` | Teste de unidade com textos ricos em emojis e acentuação complexa validando a contagem exata e o limite de 3.000 graphemes. |
| **REQ-ARG-05** | Interface de prévia obrigatória e exibição do cálculo exato de custo em INK antes da confirmação de publicação pelo participante. | MVP §3 (Argumentação); BR §4.1, §8 | `arguments` | `10-arguments` | Teste de endpoint de orçamentação/prévia retornando custo exato em INK sem efetivar débito. |
| **REQ-ARG-06** | Retirada voluntária de argumento da exibição pública pelo autor, mantendo o registro histórico e auditável sem estorno automático de INK. | MVP §3 (Argumentação); BR §4.1 | `arguments` | `10-arguments` | Teste de retirada de argumento: status visual atualizado para oculto/retirado; ausência de crédito no ledger. |
| **REQ-ARG-07** | Imutabilidade do conteúdo publicado: argumentos e respostas não sofrem edição silenciosa no MVP. | BR §4.1 | `arguments` | `10-arguments` | Teste de ausência de endpoint de alteração de conteúdo ou rejeição de mutação de texto publicado. |
| **REQ-ARG-08** | Ordenação padrão de listas de argumentos por data (recente/antiga); exibição de contagem de persuasão sem tornar-se o ranking padrão obrigatório. | BR §4.3 | `arguments` | `10-arguments` | Teste de ordenação das consultas de argumentos garantindo respeito aos filtros de ordenação cronológica. |

### 2.7 Persuasão e atribuição (`persuasion`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-PERS-01** | Apresentação de convite contextual e opcional para atribuição de influência imediatamente após uma mudança de posição confirmada. | MVP §3 (Persuasão); BR §5 | `persuasion` | `11-persuasion` | Teste de fluxo de mudança de posição gerando retorno com lista de argumentos elegíveis para atribuição. |
| **REQ-PERS-02** | Seleção de no mínimo zero e no máximo três argumentos que contribuíram para a mudança de posição do participante. | MVP §3 (Persuasão); BR §5, §11 (Inv 6) | `persuasion` | `11-persuasion` | Teste de caso de uso com 0, 1, 2 e 3 argumentos; teste de rejeição com erro 400 ao tentar submeter 4 ou mais argumentos. |
| **REQ-PERS-03** | Critérios estritos de elegibilidade de argumentos: pertencer à mesma arena, ter sido publicado antes da mudança e ser de autoria de outro participante. | BR §5 | `persuasion` | `11-persuasion` | Teste de validação de elegibilidade: rejeição de argumentos de outra arena, argumentos criados após a mudança ou inativos. |
| **REQ-PERS-04** | Bloqueio server-side absoluto de autoatribuição: o participante é estritamente impedido de atribuir influência a argumentos de sua própria autoria. | MVP §3 (Persuasão); BR §5.1, §11 (Inv 5) | `persuasion` | `11-persuasion` | Teste de autorização negativa tentando atribuir influência a argumento próprio (esperando falha 403 / invariante violada). |
| **REQ-PERS-05** | Preservação da privacidade do eleitor: a identidade de quem atribuiu influência permanece estritamente privada por padrão; exibição de totais válidos. | BR §5.1 | `persuasion` | `11-persuasion` | Teste de API pública garantindo que endpoints de contagem de persuasão nunca retornam a lista de IDs de votantes. |
| **REQ-PERS-06** | Cálculo e consolidação de métricas factuais de persuasão por argumento, por autor, por arena e por categoria. | MVP §3 (Persuasão); BR §5, §6 | `persuasion` | `11-persuasion` | Teste de agregação de métricas; validação da contagem correta após múltiplas atribuições válidas. |
| **REQ-PERS-07** | Métrica de manchete "pessoas influenciadas" computada contabilizando cada participante no máximo uma vez por autor em cada arena. | BR §6 | `persuasion` | `11-persuasion` | Teste de agregação com participante mudando e atribuindo duas vezes ao mesmo autor na mesma arena (contagem deve ser 1). |
| **REQ-PERS-08** | Invalidação administrativa de atribuições fraudulentas ou conspiratórias via moderação, preservando registro da decisão sem alterar dados do eleitor. | MVP §3 (Persuasão); BR §5.1, §6 | `persuasion` / `moderation` | `11-persuasion` | Teste de recálculo de métricas de persuasão excluindo atribuições marcadas como fraudulentas. |

### 2.8 Carteira e ledger de INK (`wallet`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-WAL-01** | Ledger append-only de INK implementado em PostgreSQL como sistema de registro imutável; saldos são projeções derivadas dos lançamentos. | BR §8, §11 (Inv 7); STACK §1 | `wallet` | `06-wallet-ledger` | Teste de auditoria do ledger; garantia de bloqueio de instruções `UPDATE` e `DELETE` no ledger de transações. |
| **REQ-WAL-02** | Custo de publicação fixado em exatamente 1 INK por grapheme cluster de texto do argumento, incluindo pontuações, espaços e quebras de linha. | BR §8 | `wallet` | `06-wallet-ledger` | Teste unitário de tarifação comparando número exato de graphemes com o montante debitado em INK. |
| **REQ-WAL-03** | Atomicidade estrita entre débito de INK e publicação de argumento: execução na mesma transação de banco com controle de concorrência contra double-spend. | BR §8, §11 (Inv 7); TM THR-WAL-01 | `wallet` / `arguments` | `06-wallet-ledger` | Teste de corrida concorrente (`go test -race`, 50 goroutines debitando simultaneamente) garantindo que nenhum saldo fique negativo. |
| **REQ-WAL-04** | Consumo prioritário de saldo: créditos promocionais/incluídos no plano são debitados compulsoriamente antes do saldo comprado. | BR §8 | `wallet` | `06-wallet-ledger` | Teste de débito misto: verifica que balanço incluído é zerado antes de iniciar débito no balanço comprado. |
| **REQ-WAL-05** | Extrato histórico completo e compreensível disponível ao usuário detalhando data, motivo, identificador do argumento/evento e montante de cada débito/crédito. | MVP §3 (Capacidade); BR §8 | `wallet` | `06-wallet-ledger` | Teste de consulta de extrato da carteira validando paginação e consistência matemática de saldo. |

### 2.9 Passes e entitlements (`entitlements`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-ENT-01** | Controle de saldo de Arena Passes: publicação bem-sucedida de uma nova arena consome exatamente 1 passe; rascunhos não consomem passe. | BR §9, §11 (Inv 8) | `entitlements` | `07-entitlements` | Teste transacional de consumo de passe; validação de que falha na publicação não consome o passe. |
| **REQ-ENT-02** | Distinção formal e rastreável no ledger de passes entre passes avulsos comprados e passes periódicos incluídos na assinatura Member. | BR §9 | `entitlements` | `07-entitlements` | Teste de segregação de tipos de passes e verificação de precedência de expiração em passes promocionais. |

### 2.10 Faturamento e compras (`billing`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-BIL-01** | Catálogo de compra avulsa de pacotes de INK e Arena Passes com redirecionamento para sessão segura de Stripe Checkout. | MVP §3 (Capacidade) | `billing` | `12-billing` | Teste de criação de checkout session com Stripe SDK mockado/isolado no adapter. |
| **REQ-BIL-02** | Assinatura recorrente mensal Member com concessão de franquia de INK e passes no ciclo, gerenciada via Stripe Billing / Customer Portal. | MVP §3 (Capacidade) | `billing` | `12-billing` | Teste de provisionamento de assinatura e ciclo de renovação baseado em webhook Stripe. |
| **REQ-BIL-03** | Processamento idempotente de webhooks Stripe com verificação de assinatura HMAC-SHA256 no payload bruto e gravação de `stripe_event_id` único. | MVP §3 (Capacidade); TM THR-STRIPE-01, 03 | `billing` | `12-billing` | Teste de webhook com verificação de assinatura e teste de replay com o mesmo event_id (deve retornar 200 sem duplicar saldo). |
| **REQ-BIL-04** | Tratamento explícito de reembolsos (refunds) e contestações (chargebacks) da Stripe via eventos de estorno auditados sem mutação silenciosa do histórico. | MVP §3 (Capacidade); BR §8, §9 | `billing` / `wallet` | `12-billing` | Teste de processamento de evento `charge.refunded` gerando lançamento compensatório de estorno no ledger. |

### 2.11 Moderação, denúncias e recursos (`moderation`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-MOD-01** | Submissão de denúncias por usuários autenticados com seleção de categorias tipadas de infração e justificativa textual restrita. | MVP §3 (Confiança); BR §1 | `moderation` | `13-moderation` | Teste de criação de denúncia; validação de rate limit de denúncias por usuário para conter brigading. |
| **REQ-MOD-02** | Fila interna de triagem e análise de denúncias para deliberação de moderadores humanos autorizados. | MVP §3 (Confiança); BR §1 | `moderation` | `13-moderation` | Teste de listagem e filtragem de tickets na fila de moderação por prioridade e tipo. |
| **REQ-MOD-03** | Ações formais de moderação: advertência, restrição de interação, suspensão temporária de conta e remoção de conteúdo ilícito da exibição pública. | MVP §3 (Confiança); BR §2.2, §10 | `moderation` | `13-moderation` | Teste de aplicação de sanção; verificação imediata de bloqueio de mutações para usuário suspenso. |
| **REQ-MOD-04** | Fluxo formal de recurso (`appeal`) pelo participante penalizado contra decisão de moderação, revisável por operador distinto. | MVP §3 (Confiança) | `moderation` | `13-moderation` | Teste de submissão de recurso contra sanção e teste de reavaliação com manutenção ou reversão de sanção. |

### 2.12 Auditoria e transparência (`audit` & `transparency`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-AUD-01** | Trilha de auditoria administrativa append-only registrando compulsoriamente autor, ação, justificativa, IP confiável e timestamp de toda intervenção operacional. | MVP §3 (Confiança); BR §11 (Inv 9); TM THR-ADM-02 | `audit` | `13-moderation` / `02-platform-core` | Teste de gravação atômica de log de auditoria em mutações de moderador/administrador; bloqueio de UPDATE/DELETE na trilha. |
| **REQ-TRP-01** | Páginas públicas e institucionais versionadas de constituição, regras de moderação, relatório de transparência e roadmap. | MVP §3 (Confiança) | `transparency` | `14-transparency-privacy` | Teste de rota e renderização de páginas públicas de diretrizes e transparência. |
| **REQ-TRP-02** | Garantia de reconstrução matemática de métricas de reputação e agregados a partir exclusivamente de eventos históricos válidos e auditados. | BR §11 (Inv 10) | `transparency` / `persuasion` | `14-transparency-privacy` | Teste de recomputação de métricas a partir do replay de eventos históricos válidos, comparando com projeções. |

### 2.13 Internacionalização (`platform` / `i18n`)

| ID | Descrição do Requisito | Origem | Módulo | Fase do Plano | Teste Futuro |
|---|---|---|---|---|---|
| **REQ-I18N-01** | Suporte completo na interface da plataforma aos idiomas Português do Brasil (`pt-BR`) e Inglês (`en-US`). | MVP §3 (Idiomas) | `platform/i18n` | `02-platform-core` | Teste de catálogos tipados de tradução; verificação de formatação localizada com `Intl`. |
| **REQ-I18N-02** | Associação obrigatória e imutável de um idioma específico a cada arena criada. | MVP §3 (Idiomas); BR §2 | `arenas` | `08-arenas` | Teste de validação de criação de arena com código de idioma homologado. |
| **REQ-I18N-03** | Segregação estrita de feeds públicos de arenas por idioma selecionado pelo participante ou visitante. | MVP §3 (Idiomas) | `arenas` | `08-arenas` | Teste de feed confirmando que arenas de outros idiomas não vazam na listagem filtrada. |
| **REQ-I18N-04** | Ausência explícita de mecanismos de tradução automática pelo backend no MVP para preservar precisão textual de argumentos. | MVP §3 (Idiomas), §4 | `platform` | `02-platform-core` | Validação arquitetural confirmando ausência de dependências ou serviços de tradução automatizada. |

---

## 3. Matriz de invariantes de negócio

As invariantes fundamentais de [docs/BUSINESS_RULES.md](BUSINESS_RULES.md) Seção 11 são regras invioláveis de integridade sistêmica:

| ID | Invariante do Negócio | Módulo Principal | Fase do Plano | Mecanismo de Garantia / Teste |
|---|---|---|---|---|
| **REQ-INV-01** | Dinheiro e planos pagos não alteram o peso ou o impacto de qualquer posição no agregado oficial. | `positions` / `billing` | `09-positions` | Teste de agregação confirmando pesos unitários idênticos para membros Free e Member pagantes. |
| **REQ-INV-02** | Escolhas de visitantes não entram no agregado oficial sob nenhuma circunstância. | `positions` | `09-positions` | Teste de filtro de elegibilidade em consultas agregadas excluindo registros de visitantes. |
| **REQ-INV-03** | A primeira posição confirmada por um participante em uma arena nunca é reescrita ou apagada. | `positions` | `09-positions` | Constraint de unicidade no banco e teste tentando sobrescrever posição inicial. |
| **REQ-INV-04** | Toda posição atual de um participante em uma arena deriva estritamente do seu histórico imutável de mudanças. | `positions` | `09-positions` | Teste de verificação de consistência entre a posição corrente projetada e a sequência de eventos de mudança. |
| **REQ-INV-05** | Nenhum participante pode atribuir influência a argumentos de sua própria autoria. | `persuasion` | `11-persuasion` | Invariante de domínio e teste de autorização negativa rejeitando submissão. |
| **REQ-INV-06** | No máximo três argumentos podem receber atribuição de influência por evento de mudança de posição. | `persuasion` | `11-persuasion` | Validação de domínio e constraint de tamanho de array de atribuições (máximo 3). |
| **REQ-INV-07** | A publicação de um argumento e o débito do saldo correspondente de INK formam uma unidade atômica indivisível. | `wallet` / `arguments` | `10-arguments` | Teste de concorrência com injeção de falha garantindo rollback completo de publicação se débito falhar. |
| **REQ-INV-08** | Um Arena Pass só é consumido quando a transição da arena de `draft` para `published` é bem-sucedida. | `entitlements` / `arenas` | `08-arenas` | Teste transacional validando que erro na publicação preserva o saldo de passes intacto. |
| **REQ-INV-09** | A remoção de conteúdo da exibição pública não apaga registros de auditoria ou dados necessários à integridade. | `moderation` / `audit` | `13-moderation` | Teste de remoção confirmando que registros continuam presentes em tabelas de auditoria do banco. |
| **REQ-INV-10** | Toda métrica de reputação e persuasão pode ser recalculada a partir dos fatos históricos válidos e auditados. | `persuasion` / `transparency` | `11-persuasion` | Teste de reconciliação matemática executando recálculo do zero a partir dos eventos de atribuição. |

---

## 4. Tabela de cobertura exaustiva dos documentos de origem

Esta seção comprova que cada item do escopo funcional de [docs/MVP.md](MVP.md) e das regras de [docs/BUSINESS_RULES.md](BUSINESS_RULES.md) possui correspondência formal:

| Item do Documento de Origem | Referência Original | Requisitos Mapeados |
|---|---|---|
| Cadastro e login | MVP §3 (Conta e perfil) | `REQ-AUTH-01` |
| Verificação de email | MVP §3 (Conta e perfil); BR §1, §7 | `REQ-AUTH-02` |
| Sessões seguras e logout | MVP §3 (Conta e perfil) | `REQ-AUTH-03` |
| Recuperação de senha | MVP §3 (Conta e perfil) | `REQ-AUTH-04` |
| Username público | MVP §3 (Conta e perfil); BR §1 | `REQ-PROF-01` |
| Locale e idioma do usuário | MVP §3 (Conta e perfil); BR §1 | `REQ-PROF-02` |
| Perfil com métricas factuais | MVP §3 (Conta e perfil); BR §6 | `REQ-PROF-03` |
| Exportação e exclusão de conta | MVP §3 (Conta e perfil); BR §10 | `REQ-PRIV-01` |
| Feed básico por idioma | MVP §3 (Descoberta) | `REQ-DISC-01` |
| Página pública indexável de Arena | MVP §3 (Descoberta); BR §2 | `REQ-DISC-02` |
| Filtros de categoria e status | MVP §3 (Descoberta); BR §2.2 | `REQ-DISC-03` |
| Rascunho sem custo de passe | MVP §3 (Arena); BR §2.2, §9 | `REQ-ARN-01` |
| Publicação com consumo de Arena Pass | MVP §3 (Arena); BR §2.2, §9 | `REQ-ARN-02` |
| Afirmação, contexto, categoria, encerramento | MVP §3 (Arena); BR §2, §2.1 | `REQ-ARN-03` |
| Estados de ciclo de vida da Arena | MVP §3 (Arena); BR §2.2 | `REQ-ARN-04` |
| Afirmação imutável após publicação | MVP §3 (Arena); BR §2.3 | `REQ-ARN-05` |
| Identificador perene e slug amigável | BR §2.3 | `REQ-ARN-06` |
| Exportação pública versionada de Arena | MVP §3 (Arena); BR §2.3 | `REQ-ARN-07` |
| Escolha local de visitante para liberar agregado | MVP §3 (Posição); BR §3.1 | `REQ-POS-01` |
| Confirmação autenticada da posição inicial | MVP §3 (Posição); BR §3, §3.1 | `REQ-POS-02` |
| Posição inicial imutável (fato histórico) | MVP §3 (Posição); BR §3.2 | `REQ-POS-03` |
| Posições e mudanças individuais privadas | MVP §3 (Posição); BR §3.2 | `REQ-POS-04` |
| Mudança imutável de posição | MVP §3 (Posição); BR §3.3 | `REQ-POS-05` |
| Proibição de mudança para valor idêntico | BR §3.3 | `REQ-POS-06` |
| Arenas fechadas não aceitam novas posições | BR §2.2, §3.3 | `REQ-POS-07` |
| Agregados com total e aviso metodológico | MVP §3 (Posição); BR §7 | `REQ-POS-08` |
| Bloqueio de agregado pré-escolha | BR §3.2; MVP §5 | `REQ-POS-09` |
| Argumento a favor, contra ou contextual | MVP §3 (Argumentação); BR §4 | `REQ-ARG-01` |
| Respostas em 1 nível recursivo | MVP §3 (Argumentação); BR §4 | `REQ-ARG-02` |
| Fontes por URL e descrição | MVP §3 (Argumentação); BR §4.2 | `REQ-ARG-03` |
| Limite de 3.000 grapheme clusters | MVP §3 (Argumentação); BR §4 | `REQ-ARG-04` |
| Prévia e custo antes de publicar | MVP §3 (Argumentação); BR §4.1, §8 | `REQ-ARG-05` |
| Retirada pelo autor sem estorno silencioso | MVP §3 (Argumentação); BR §4.1 | `REQ-ARG-06` |
| Argumentos imutáveis (sem edição silenciosa) | BR §4.1 | `REQ-ARG-07` |
| Ordenação cronológica inicial | BR §4.3 | `REQ-ARG-08` |
| Convite à atribuição após mudança | MVP §3 (Persuasão); BR §5 | `REQ-PERS-01` |
| Zero a três argumentos elegíveis | MVP §3 (Persuasão); BR §5 | `REQ-PERS-02` |
| Elegibilidade de argumentos para atribuição | BR §5 | `REQ-PERS-03` |
| Bloqueio de autoatribuição | MVP §3 (Persuasão); BR §5.1 | `REQ-PERS-04` |
| Privacidade de quem atribuiu | BR §5.1 | `REQ-PERS-05` |
| Métricas por argumento, autor, arena | MVP §3 (Persuasão); BR §6 | `REQ-PERS-06` |
| Métrica "pessoas influenciadas" única por autor | BR §6 | `REQ-PERS-07` |
| Invalidação de atribuições fraudulentas | MVP §3 (Persuasão); BR §5.1, §6 | `REQ-PERS-08` |
| Ledger append-only de INK | BR §8; STACK §1 | `REQ-WAL-01` |
| Custo de 1 INK por grapheme cluster | BR §8 | `REQ-WAL-02` |
| Consumo e publicação indivisíveis | BR §8 | `REQ-WAL-03` |
| Precedência de consumo de saldos | BR §8 | `REQ-WAL-04` |
| Extrato detalhado de créditos e consumos | MVP §3 (Capacidade); BR §8 | `REQ-WAL-05` |
| Arena Pass consumido por publicação | BR §9 | `REQ-ENT-01` |
| Distinção de passes comprados vs assinatura | BR §9 | `REQ-ENT-02` |
| Compra de pacotes de INK e passes via Stripe | MVP §3 (Capacidade) | `REQ-BIL-01` |
| Assinatura Member mensal | MVP §3 (Capacidade) | `REQ-BIL-02` |
| Processamento idempotente de webhooks Stripe | MVP §3 (Capacidade) | `REQ-BIL-03` |
| Suporte a estornos e chargebacks no ledger | MVP §3 (Capacidade); BR §8 | `REQ-BIL-04` |
| Denúncias por participantes | MVP §3 (Confiança); BR §1 | `REQ-MOD-01` |
| Fila e triagem de moderação | MVP §3 (Confiança); BR §1 | `REQ-MOD-02` |
| Sanções e restrições de moderação | MVP §3 (Confiança); BR §2.2, §10 | `REQ-MOD-03` |
| Recurso contra decisão de moderação | MVP §3 (Confiança) | `REQ-MOD-04` |
| Trilha de auditoria administrativa | MVP §3 (Confiança); BR §11 | `REQ-AUD-01` |
| Páginas de constituição e transparência | MVP §3 (Confiança) | `REQ-TRP-01` |
| Reconstrução matemática de métricas | BR §11 (Inv 10) | `REQ-TRP-02` |
| Idiomas pt-BR e en-US | MVP §3 (Idiomas) | `REQ-I18N-01` |
| Idioma fixado por arena | MVP §3 (Idiomas); BR §2 | `REQ-I18N-02` |
| Feeds separados por idioma | MVP §3 (Idiomas) | `REQ-I18N-03` |
| Nenhuma tradução automática no MVP | MVP §3 (Idiomas), §4 | `REQ-I18N-04` |
| Invariantes 1 a 10 de regras de negócio | BR §11 | `REQ-INV-01` a `REQ-INV-10` |

---

## 5. Itens explicitamente excluídos do MVP (Não-Requisitos)

Conforme [docs/MVP.md](MVP.md) Seção 4, os seguintes itens estão expressamente fora do escopo funcional do MVP e não possuem implementação autorizada:
- `NON-REQ-01`: Mensagens diretas, chat privado ou canais de comunicação paralelos entre participantes.
- `NON-REQ-02`: Mecanismos de seguidores, listas de amigos ou grafo social pessoal.
- `NON-REQ-03`: Curtidas, reações genéricas (emojis em posts) ou contadores de popularidade vazia.
- `NON-REQ-04`: Áudio, vídeo, livestreams ou transmissões de debate síncronas.
- `NON-REQ-05`: Aplicativo móvel nativo (iOS/Android).
- `NON-REQ-06`: Modelos de Inteligência Artificial para julgar debates, resumir argumentos ou ranquear participantes.
- `NON-REQ-07`: Tradução automática no browser ou servidor.
- `NON-REQ-08`: Tokens em blockchain, criptomoedas, apostas, predições financeiras ou especulação.
- `NON-REQ-09`: Pagamento ou divisão de receita com autores de argumentos.
- `NON-REQ-10`: Algoritmos de recomendação secreta ou feed algorítmico personalizado.
- `NON-REQ-11`: Múltiplos níveis de planos pagos além da assinatura Member básica.
- `NON-REQ-12`: Contas organizacionais, equipes corporativas ou gerenciamento multiusuário de arenas.
- `NON-REQ-13`: API pública genérica aberta a terceiros além do endpoint de exportação versionada.
- `NON-REQ-14`: Integração com identidade governamental ou sistemas biométricos pesados.
