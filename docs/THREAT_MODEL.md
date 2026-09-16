# Modelo de ameaças (Threat Model)

**Status:** padrão obrigatório

**Referência de segurança:** [docs/SECURITY.md](SECURITY.md) · [SECURITY.md](../SECURITY.md) · OWASP ASVS 5 / STRIDE

**Última revisão:** 2026-09-16

---

## 1. Visão geral e metodologia

Este documento estabelece o modelo formal de ameaças do Goyim Arena para orientar a implementação do backend, dos contratos de API e do harness web mínimo. A análise baseia-se na metodologia **STRIDE** (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege) aplicada aos fluxos de dados e ativos mais críticos do sistema.

### Princípios mandatórios
- Toda ameaça identificada possui um identificador único `THR-*`.
- **Gate de segurança:** nenhuma ameaça com severidade **Crítica** pode ter apenas "monitorar" ou "auditar" como mitigação; é mandatório definir mitigação arquitetural preventiva e teste de segurança verificável.
- Falhas de validação de ameaças bloqueiam o pipeline de release.

---

## 2. Ativos críticos a proteger (Assets)

1. **Credenciais e segredos de autenticação:**
   - Hashes Argon2id de senhas de usuários.
   - Tokens de sessão ativos e hashes de tokens.
   - Tokens de redefinição de senha e de confirmação de email.
   - Segredos de integração (webhook secret da Stripe, chaves de API Resend/Turnstile).
2. **Saldo e integridade financeira:**
   - Ledger append-only de INK (tabela imutável de transações).
   - Registros de propriedade de Arena Passes e assinaturas Member.
   - Histórico de eventos de faturamento e webhooks da Stripe.
3. **Integridade das arenas e do debate:**
   - Registros de posições individuais (inicial, intermediária e final).
   - Votos e métricas de atribuição de persuasão.
   - Conteúdo de arenas, argumentos, respostas e fontes submetidas.
4. **Dados privados e controles de privacidade do titular:**
   - Endereço de email e preferências privadas de perfil.
   - Posição individual de voto antes da revelação coletiva.
   - Identidade de denunciantes em relatórios de moderação.
5. **Trilha de auditoria e moderação:**
   - Histórico imutável de decisões de moderadores e sanções aplicadas.
   - Logs de ações administrativas sensíveis.
6. **Infraestrutura e disponibilidade:**
   - Instância PostgreSQL 18 e consistência das réplicas.
   - Servidor HTTP da aplicação Go e workers assíncronos.
   - Reverse proxy Caddy e regras de WAF/CDN na borda Cloudflare.

---

## 3. Atores de ameaça (Threat Actors)

- **A-01: Visitante anônimo / Atacante de internet:**
  - Sem credenciais válidas. Tenta enumeração de endpoints, brute-force de senhas, scraping de conteúdo, cache deception e negação de serviço.
- **A-02: Usuário autenticado básico:**
  - Possui conta registrada. Tenta violação de autorização horizontal (IDOR), gasto duplo de INK, submissão de argumentos sem débito correspondente e manipulação de votos.
- **A-03: Rede adversária / Fazenda de bots (Sybil / Brigading):**
  - Criação coordenada de múltiplas contas automatizadas para esgotar bônus promocionais de INK, manipular artificialmente a persuasão de uma arena ou silenciar oponentes via denúncias em massa.
- **A-04: Usuário com privilégio comprometido (Moderador / Admin):**
  - Conta com privilégios de moderação ou administração que sofre sequestro de credencial ou abuso deliberado de autoridade sem rastreabilidade.
- **A-05: Adversário de rede (Man-in-the-Middle / Proxy hostil):**
  - Interceptação de tráfego não criptografado, replay de requisições de pagamento ou falsificação de webhooks externos.
- **A-06: Fornecedor ou dependência comprometida (Supply Chain):**
  - Injeção de código malicioso via pacotes ou containers de terceiros.

---

## 4. Fronteiras de confiança (Trust Boundaries)

```text
[ Browser / Cliente ] (Não confiável)
       │ (TB-01: Internet / HTTPS / Turnstile)
       ▼
[ Cloudflare CDN & WAF ] (Borda de mitigação)
       │ (TB-02: TLS Mútuo / Headers validados)
       ▼
[ Caddy Reverse Proxy ] (Proxy de terminação e compressão)
       │ (TB-03: Rede local / Loopback / HTTP limpo)
       ▼
[ Go Monolith: Adapters Inbound ] (Decodificação e validação sintática)
       │ (TB-04: Portas de Aplicação - apenas DTOs e Comandos tipados)
       ▼
[ Go Monolith: Application & Domain Core ] (Regras de negócio e invariantes)
       │ (TB-05: Driver PostgreSQL / Conexão autenticada isolada)
       ▼
[ PostgreSQL 18 Database ] (Sistema de registro ACID - Schema app)
       ▲
       │ (TB-06: Assinatura criptográfica HMAC-SHA256 em Webhooks)
[ Gateways Externos: Stripe / Resend ]
```

- **TB-01 (Usuário ↔ Borda):** Todo payload, header, cookie e IP de origem é estritamente não confiável.
- **TB-02 (Borda ↔ Caddy):** Apenas tráfego filtrado pelo WAF e com cabeçalhos de conexão específicos é aceito; IPs reais são extraídos exclusivamente dos headers assinados do Cloudflare.
- **TB-03 (Caddy ↔ Backend Go):** A aplicação Go não confia cegamente no proxy; valida integridade de transporte, CSRF, cookies e tamanho de payload.
- **TB-04 (Adapters ↔ Domínio Go):** Nenhum tipo externo, framework ou struct de transporte ultrapassa a fronteira da aplicação.
- **TB-05 (Backend Go ↔ PostgreSQL):** Queries parametrizadas via sqlc; usuário do banco possui privilégios mínimos necessários ao schema `app`.
- **TB-06 (Gateways Externos ↔ Backend Go):** Webhooks são rejeitados se a assinatura criptográfica e a tolerância temporal falharem.

---

## 5. Análise STRIDE por fluxo crítico

### 5.1 Autenticação e gestão de sessões (Auth)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-AUTH-01** | Sequestro de sessão via roubo de token em trânsito ou browser | Spoofing | A-01, A-05 | **Crítica** | Sessões baseadas em tokens opacos de 256 bits com hash SHA-256 no banco; cookies com atributos `HttpOnly`, `Secure`, `SameSite=Lax` e prefixo `__Host-` em produção; rotação compulsória de sessão em login, logout e alteração de senha; expiração inativa (24h) e absoluta (14 dias). | Teste de integração de rotação de token pós-login; validação de headers de cookie; teste de rejeição imediata de token antigo após logout. | Baixo |
| **THR-AUTH-02** | Força bruta em credenciais e enumeração de usuários em login/recuperação | Information Disclosure / DoS | A-01 | **Média** | Respostas de tempo e formato uniformes em login e esqueci-minha-senha (RFC 9457 idêntico); rate limiting por IP/conta; desafio Turnstile em falhas consecutivas; senhas com Argon2id com parâmetros calibrados. | Teste de resposta idêntica (status e Problem Details) para emails cadastrados e inexistentes; teste de bloqueio por rate limiter (429). | Baixo |
| **THR-AUTH-03** | Falsificação de requisição cross-site em mutações de conta (CSRF) | Tampering / Spoofing | A-01, A-02 | **Alta** | Token CSRF criptográfico de uso associado à sessão exigido em métodos `POST`, `PUT`, `PATCH`, `DELETE`; validação estrita de cabeçalho `Origin` e `Sec-Fetch-Site`. | Teste de mutação sem cabeçalho CSRF ou com token adulterado (esperando 403 Forbidden); validação de rejeição de requisições cross-origin. | Baixo |
| **THR-AUTH-04** | Replay ou força bruta de token de recuperação de senha | Tampering / Elevation | **Alta** | Token criptográfico CSPRNG de uso estritamente único; expiração em 15 minutos; invalidação imediata no primeiro uso ou se novo token for solicitado; hash SHA-256 no banco. | Teste de tentativa de reutilização de token já utilizado (esperando falha); teste de submissão de token expirado (esperando erro). | Baixo |

### 5.2 Carteira e concorrência (Wallet / INK)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-WAL-01** | Gasto duplo de saldo INK em submissões concorrentes | Tampering | A-02, A-03 | **Crítica** | Débito de INK e publicação de argumento executados na mesma transação atômica do PostgreSQL; lock pessimista a nível de linha (`SELECT ... FOR UPDATE` no registro de saldo) ou constraint estrita `CHECK (balance >= 0)`; idempotency key obrigatória na submissão de argumentos. | Teste de estresse com concorrência massiva (`go test -race` e 50 goroutines debitando a mesma conta com saldo para 1 operação); confirmação de exatamente 1 sucesso e 49 rejeições. | Baixo |
| **THR-WAL-02** | Alteração arbitrária de saldo ou manipulação direta do ledger | Tampering | A-02, A-04 | **Crítica** | O ledger de INK é estritamente append-only; permissões do banco proíbem `UPDATE` e `DELETE` na tabela de transações do ledger para o usuário de aplicação; valores monetários e saldos expressos exclusivamente em inteiros (`bigint`); rotina periódica de reconciliação matemática entre lançamentos do ledger e projeções de saldo. | Teste de integração executando instruções SQL `UPDATE` ou `DELETE` no ledger (deve falhar por permissão); teste de integridade de conciliação matemática. | Baixo |
| **THR-WAL-03** | Exploração de saldo promocional através de criação massiva de contas (Sybil Farming) | Elevation / Tampering | A-03 | **Alta** | Grants de INK atrelados a requisitos de validação prévia de conta (email confirmado, desafio Turnstile); INK promocional é não sacável e não transferível entre contas de usuários; limites de ritmo por IP e heurística de contenção de automação. | Teste de tentativa de consumo de grant por conta não confirmada (esperando 403); validação de ausência de endpoint de transferência direta de INK. | Médio |

### 5.3 Cobrança e pagamentos (Stripe)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-STRIPE-01** | Falsificação de webhook da Stripe forjando evento de pagamento aprovado | Spoofing / Tampering | A-01, A-05 | **Crítica** | Verificação criptográfica de assinatura HMAC-SHA256 (`Stripe-Signature`) contra o payload bruto (`r.Body` sem deserialização prévia) e o segredo `STRIPE_WEBHOOK_SECRET`; tolerância de timestamp estrita (máximo de 300s); registro do `stripe_event_id` único em transação com constraint `UNIQUE`. | Teste de contrato do webhook enviando payload sem assinatura, com assinatura adulterada ou com timestamp antigo (deve falhar com 400/401); auditoria de integridade. | Baixo |
| **THR-STRIPE-02** | Concessão de benefício baseada em visita à URL de sucesso no browser | Elevation / Tampering | A-02 | **Crítica** | A página de sucesso do frontend é meramente informativa; concessão de Arena Pass ou crédito de INK ocorre **exclusivamente** no processamento do webhook assíncrono oficial autenticado; autorização de recursos consulta o estado gravado no PostgreSQL, nunca parâmetros da URL. | Teste de navegação direta para `/checkout/success` sem webhook correspondente, confirmando que nenhum passe ou crédito foi provisionado. | Baixo |
| **THR-STRIPE-03** | Reenvio de webhook Stripe duplicando créditos ou passes concedidos | Repudiation / Tampering | A-01, A-05 | **Alta** | Processamento idempotente baseado na tabela transacional `processed_stripe_events`; reenvios do mesmo `event_id` são identificados imediatamente e retornam HTTP 200 sem reexecutar mutações no ledger. | Teste de integração enviando o mesmo evento de webhook duas vezes consecutivas e confirmando que o ledger registrou apenas uma transação. | Baixo |

### 5.4 Persuasão e antiabuso (Persuasion)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-PERS-01** | Manipulação de persuasão por anel de contas cúmplices (Collusion / Self-voting) | Tampering / Elevation | A-02, A-03 | **Alta** | Bloqueio server-side de voto de persuasão em argumentos de própria autoria; validação de mudança real de posição entre abertura e fechamento; restrição de elegibilidade por tempo mínimo de permanência na arena; penalização de peso algorítmico em padrões repetidos de reciprocidade. | Teste unitário de caso de uso rejeitando autoatribuição de persuasão e votos com posição idêntica; teste de validação de elegibilidade de tempo. | Médio |
| **THR-PERS-02** | Vazamento prematuro da distribuição agregada de posições gerando efeito manada | Information Disclosure | A-01, A-02 | **Média** | O agregado de posições de uma arena ativa é estritamente ocultado na API pública antes do registro da posição inicial do usuário; payload da API mascara contadores de votos para visitantes não posicionados; respostas protegidas contra cache compartilhado. | Teste de contrato da rota de detalhes da arena confirmando que o payload para usuário não posicionado oculta os agregados de posições. | Baixo |

### 5.5 Moderação e integridade (Moderation)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-MOD-01** | Censura de conteúdo via denúncias massivas coordenadas (Brigading) | Denial of Service | A-01, A-03 | **Alta** | Nenhuma remoção de conteúdo ou banimento ocorre de forma automática baseada em volume de denúncias; denúncias são enfileiradas para triagem por moderadores humanos; rate limit de denúncias por usuário e IP; abuso reiterado de denúncias falsas gera penalidade à conta denunciante. | Teste simulando submissão massiva de denúncias contra o mesmo argumento e confirmando que o conteúdo permanece acessível com status original até julgamento humano. | Baixo |
| **THR-MOD-02** | Ações de moderação arbitrárias, abusivas ou não auditadas | Elevation / Repudiation | A-04 | **Alta** | Todas as mutações de moderação geram evento imutável obrigatório em `moderation_actions` gravando moderador, alvo, justificativa tipada e snapshot de estado; histórico de sanções é append-only; disponibilização de fluxo formal de recurso (`appeal`) revisável por outro operador. | Teste de integração de caso de uso de moderação garantindo que tentativa de suspensão sem justificativa ou sem gravação na trilha de auditoria aborta a transação. | Baixo |
| **THR-MOD-03** | Vazamento de identidade do denunciante ou de dados de denúncia | Information Disclosure | A-02, A-04 | **Média** | Endpoints públicos e de usuários comuns nunca expõem identificador de denunciantes nem dados de tickets de denúncia; logs de aplicação redigem texto livre de queixas; acesso restrito por política de autorização em tempo de execução. | Teste de autorização comprovando que usuário comum recebe 403/404 ao tentar acessar detalhes de tickets de denúncia de terceiros. | Baixo |

### 5.6 Camada de cache (Cache / Edge)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-CACHE-01** | Vazamento de dados privados via Web Cache Deception ou cache em CDN | Information Disclosure | A-01, A-05 | **Crítica** | Toda rota autenticada (`/api/v1/me/*`, auth, wallet, faturamento, admin) emite compulsoriamente cabeçalhos `Cache-Control: private, no-store, no-cache, must-revalidate` e `Pragma: no-cache`; respostas com cabeçalho `Set-Cookie` nunca são cacheadas pelo Caddy ou Cloudflare; URLs públicas são rigidamente isoladas de rotas privadas. | Teste automatizado de respostas HTTP autenticadas validando presença de `no-store` e ausência de headers permissivos de cache (`public`, `s-maxage`). | Baixo |
| **THR-CACHE-02** | Envenenamento de cache (Cache Poisoning) via cabeçalhos HTTP forjados | Tampering | A-01 | **Alta** | Caddy e o backend Go ignoram cabeçalhos não confiáveis (`X-Forwarded-Host`, `Host` arbitrário) na construção de URLs canônicas ou redirecionamentos; URLs base são estáticas e fixadas na configuração de bootstrap; rotas públicas cacheadas utilizam cabeçalhos explícitos `Vary: Accept-Encoding, Accept`. | Teste de segurança enviando requisições com `X-Forwarded-Host: evil.com` e confirmando que links e respostas gerados não contêm o host injetado. | Baixo |

### 5.7 Administração e operações (Admin / Operations)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação Arquitetural Preventiva | Teste de Segurança | Risco Residual |
|---|---|---|---|---|---|---|---|
| **THR-ADM-01** | Acesso indevido a painel administrativo por escalada de privilégios | Elevation of Privilege | A-01, A-02 | **Crítica** | Rotas administrativas isoladas sob namespace `/api/v1/admin/*` com middleware dedicado de autorização server-side (`RequireRole(RoleAdmin)`); verificação no banco em cada requisição (não confia em cookies ou claims isoladas); segundo fator de autenticação (MFA) obrigatório para todas as contas operacionais antes do beta público. | Teste de matriz de autorização negativa executando todas as rotas administrativas como anônimo e como usuário comum (deve retornar 401 e 403 em 100% dos endpoints). | Baixo |
| **THR-ADM-02** | Operações administrativas financeiras ou de sistema sem rastreabilidade | Repudiation / Tampering | A-04 | **Crítica** | Ajustes manuais de saldo, encerramento forçado de arenas e alterações de configuração exigem justificativa formal, transação atômica e registro síncrono em `audit_events`; tabela de auditoria possui restrições SQL impedindo `UPDATE` e `DELETE`. | Teste de integração de comando administrativo confirmando que se a gravação do registro de auditoria falhar, a mutação administrativa sofre rollback total. | Baixo |

---

## 6. Matriz de risco residual

| Severidade Inicial | Ameaças Identificadas | Mitigação Arquitetural Preventiva | Risco Residual Final |
|---|---|---|---|
| **Crítica** | 8 ameaças (`THR-AUTH-01`, `THR-WAL-01`, `THR-WAL-02`, `THR-STRIPE-01`, `THR-STRIPE-02`, `THR-CACHE-01`, `THR-ADM-01`, `THR-ADM-02`) | Transações atômicas com locks, assinaturas HMAC, cookies `__Host-`, append-only ledgers, `Cache-Control: private, no-store`, autorização server-side estrita com MFA e auditoria compulsória. | **Baixo** |
| **Alta** | 7 ameaças (`THR-AUTH-03`, `THR-AUTH-04`, `THR-WAL-03`, `THR-STRIPE-03`, `THR-PERS-01`, `THR-MOD-01`, `THR-MOD-02`, `THR-CACHE-02`) | CSRF tokens, CSPRNG de uso único, idempotência de eventos, anti-collusion server-side, triagem humana e cabeçalhos fixados. | **Baixo / Médio** |
| **Média** | 4 ameaças (`THR-AUTH-02`, `THR-PERS-02`, `THR-MOD-03`, etc.) | Rate limiting, Problem Details uniforme, ocultação pré-voto de agregados, mascaramento de logs e permissões mínimas. | **Baixo** |

---

## 7. Rastreabilidade com testes e checklist de segurança

Cada ameaça deste documento mapeia diretamente para os testes automatizados descritos em [docs/SECURITY.md](SECURITY.md) e nos futuros gates do `Makefile`:
- `make test-unit`: valida invariantes de domínio, expiração de tokens e formatação de erros.
- `make test-contract`: valida headers de cache (`no-store`), formato RFC 9457 e assinaturas de webhooks.
- `make test-race`: valida prevenção de double-spend (`THR-WAL-01`) e concorrência no ledger.
- `make test-security`: executa a matriz de autorização negativa (`THR-ADM-01`) e testes de CSRF (`THR-AUTH-03`).
