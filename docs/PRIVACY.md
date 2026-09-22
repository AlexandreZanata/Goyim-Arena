# Privacidade e LGPD

**Status:** princípios e requisitos de produto; não substitui revisão jurídica

**Objetivo:** coletar o mínimo necessário e separar identidade privada de atividade pública

## 1. Categorias de dados

### Públicos por natureza do produto

- username e identificador público;
- Arenas, argumentos, respostas e fontes publicados;
- agregados de posições e mudanças com limiares de privacidade;
- contagens válidas de influência recebida por argumento e autor;
- datas e status públicos necessários à auditabilidade.

### Privados

- email e estado de verificação;
- informações de autenticação;
- posições individuais e histórico de mudanças;
- identidade de quem fez uma atribuição de influência;
- identificadores do provedor de pagamento;
- histórico financeiro necessário a suporte e obrigação legal;
- solicitações de privacidade e suporte;
- rascunhos não publicados.

### Restritos de segurança e moderação

- denúncias e notas internas;
- sinais de fraude, IP e dispositivo;
- trilhas administrativas;
- evidências preservadas por obrigação;
- dados usados para prevenir abuso.

Dados restritos nunca integram perfil ou exportação pública.

## 2. Minimização

No MVP, pedir apenas email, username e locale, além do que o provedor de pagamento exigir no checkout. Não exigir CPF, nome completo, endereço ou telefone por conveniência. Se obrigação fiscal exigir algum dado, preferir coleta e custódia pelo provedor apropriado e documentar a finalidade.

## 3. Transparência no momento da ação

Antes de confirmar uma ação, a interface deve diferenciar o que será público do que será usado apenas em agregados. Argumentos são públicos e históricos; posições, mudanças e autoria de atribuições são privadas por padrão, embora alimentem métricas públicas agregadas. Consentimento não deve ser usado como base genérica para tudo; cada tratamento precisa de finalidade e base legal revisadas com assessoria adequada.

## 4. Direitos do titular

Disponibilizar processo para:

- acessar dados pessoais;
- corrigir dados inexatos;
- alterar username segundo regras e histórico necessário;
- exportar dados em formato legível por máquina;
- solicitar exclusão da conta;
- obter informações sobre tratamento;
- contestar decisão automatizada, caso seja introduzida no futuro.

Pedidos devem ser autenticados sem exigir dados excessivos.

## 5. Exclusão e integridade pública

Excluir conta não é sinônimo de apagar todo registro público imediatamente. Para preservar coerência de conversas e interesses legítimos, conteúdo público pode permanecer com autoria anonimizada, desde que essa possibilidade seja informada antes da publicação e revisada juridicamente.

Dados privados sem obrigação de retenção devem ser eliminados. Registros financeiros, antifraude ou legais podem ter retenção limitada e acesso restrito.

### Cronograma de retenção

O cronograma é executável: a política vive em `internal/profiles/domain/retention.go`, é aplicada por um job que roda uma vez por classe e registra no livro `app.retention_runs` apenas contagens, classe e instantes — nunca conteúdo, identificador de titular ou payload. Cada classe tem uma ação e uma janela contada a partir do instante em que o registro se tornou terminal (usado, expirado ou revogado); o limite é inclusivo.

| Classe | Dados | Ação | Janela |
|---|---|---|---|
| `tokens` | hashes dos tokens de verificação de email e de recuperação de senha | apagar | 30 dias após o uso ou a expiração |
| `sessions` | sessões de servidor revogadas ou expiradas | apagar | 30 dias após a revogação ou a expiração |
| `referential_logs` | trilha administrativa append-only (`app.audit_events`), que referencia titular, alvo e motivo | reter como evidência | indefinida |
| `exports` | documento da exportação pessoal | apagar o documento e expirar o registro (o registro permanece) | 24 horas após o link expirar; um pedido nunca gerado expira 24 horas após o pedido |
| `abuse_signals` | IP de origem e user agent registrados na sessão | anonimizar (os campos passam a nulo; a linha permanece até a janela da classe `sessions`) | 7 dias após o término da sessão |
| `billing` | compras, assinaturas, reembolsos, eventos do provedor e reconciliação | reter como evidência financeira | indefinida |

Regras que acompanham o cronograma:

- **retenção legal e contratual:** uma retenção ativa (`app.retention_holds`) nomeia uma classe e um titular, ou a classe inteira, e suspende a ação sobre os registros cobertos; retenções são evidência, nunca são apagadas e a liberação é única e datada;
- **classes são independentes:** uma retenção na classe `sessions` preserva a linha da sessão, mas não impede a anonimização do IP e do user agent, que pertencem à classe `abuse_signals`;
- **idempotência:** repetir a execução no mesmo instante resolve o registro já gravado, e executar mais tarde encontra apenas o que ainda está fora da janela;
- **provas:** `internal/profiles/domain/retention_test.go`, `internal/profiles/application/retention_test.go`, `internal/platform/postgres/retention_schema_test.go` e `internal/profiles/adapters/postgres/retention_test.go` cobrem limites exatos, retenção legal simulada, idempotência e contagens; qualquer mudança de janela ou ação é uma mudança de política e exige revisão jurídica registrada;
- **ratificação:** as janelas acima não são apenas a implementação atual — foram ratificadas pelo proprietário como política em vigor em [GOVERNANCE.md](GOVERNANCE.md) (`retention-policy`), e mudar uma delas é decisão nova registrada lá.

## 6. Exportações

Há duas exportações distintas:

- **Exportação pessoal:** entregue ao titular autenticado e pode conter seus dados privados.
- **Exportação pública da Arena:** contém somente informações já públicas, versão de esquema e aviso metodológico.

Emails, identificadores de pagamento, IPs, sinais de dispositivo, notas de moderação e payloads de pagamento jamais entram em exportações públicas.

## 7. Analytics e observabilidade

Não enviar a ferramentas de analytics:

- email;
- texto de rascunhos;
- conteúdo restrito de denúncias;
- payload completo de pagamento;
- IP ou identificador persistente sem necessidade e avaliação;
- segredos.

Eventos devem usar identificadores pseudônimos e propriedades mínimas. Gravação de sessão, se algum dia considerada, exige avaliação separada e nunca deve capturar campos sensíveis por padrão.

## 8. Internacionalização e fornecedores

Os mercados de lançamento estão **decididos**: Brasil **e** internacional desde o beta, em português do Brasil e inglês dos Estados Unidos ([GOVERNANCE.md](GOVERNANCE.md), `launch-markets`). Como o beta já atende fora do Brasil, mapear onde os dados são processados, os mecanismos de transferência internacional e as responsabilidades de cada fornecedor deixa de ser pré-requisito apenas de operar no exterior e passa a ser pré-requisito do **próprio beta**, junto da publicação da lista de subprocessadores e do aviso de mudanças relevantes.

## 9. Crianças e adolescentes

A idade mínima é **dezoito anos**, em todos os mercados, sem exceção por mercado e sem fluxo para menores — decisão do proprietário registrada em [GOVERNANCE.md](GOVERNANCE.md) (`age-minimum`). O serviço não direciona a menores, não cria fluxos que incentivem sua participação e não coleta data de nascimento para checá-la: a declaração vive no cadastro e nos termos, e a verificação é responsabilidade de quem os aceita.

## 10. Checklist antes do beta

Cada item abaixo aponta para a decisão que o governa em [GOVERNANCE.md](GOVERNANCE.md); os que dependem de decisão ainda **não tomada** são bloqueios, e o portão `make release-gate` fica vermelho enquanto existirem.

- inventário de dados e finalidades;
- base legal por tratamento;
- política de retenção com o cronograma executável revisado juridicamente — a política está ratificada (`retention-policy`), a revisão jurídica é **pendência**;
- canal para titulares: alias de e-mail dedicado, criado e mantido pelo proprietário (`data-subject-channel`) — **pendência** até o alias existir e ser publicado nos dois idiomas;
- contratos e subprocessadores — **pendência**: a lista precisa ser publicada antes do beta, porque o beta já atende fora do Brasil;
- resposta a incidentes;
- procedimento de exclusão e anonimização;
- aviso de privacidade em pt-BR e en-US;
- termos de uso e regras de conteúdo — **bloqueio** (`terms-of-use`): falta decidir se o beta público os exige publicados nos dois idiomas, quem os redige e sob que revisão jurídica;
- revisão jurídica nos mercados de lançamento — os mercados são Brasil e internacional (`launch-markets`).
