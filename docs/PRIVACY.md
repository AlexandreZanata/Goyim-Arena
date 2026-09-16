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

Dados privados sem obrigação de retenção devem ser eliminados. Registros financeiros, antifraude ou legais podem ter retenção limitada e acesso restrito. O cronograma exato de retenção deve ser definido antes do beta público.

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

Antes de operar fora do Brasil, mapear onde dados são processados, mecanismos de transferência internacional e responsabilidades de cada fornecedor. Publicar lista de subprocessadores e avisar mudanças relevantes.

## 9. Crianças e adolescentes

A idade mínima e o tratamento de menores são questões bloqueadoras para o beta público. Até revisão jurídica, a recomendação conservadora é não direcionar o serviço a crianças e não criar fluxos que incentivem sua participação.

## 10. Checklist antes do beta

- inventário de dados e finalidades;
- base legal por tratamento;
- política de retenção;
- canal para titulares;
- contratos e subprocessadores;
- resposta a incidentes;
- procedimento de exclusão e anonimização;
- aviso de privacidade em pt-BR e en-US;
- termos de uso e regras de conteúdo;
- revisão jurídica nos mercados de lançamento.
