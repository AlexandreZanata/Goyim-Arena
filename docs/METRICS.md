# Métricas

**Status:** definições iniciais

**Regra:** métricas servem ao aprendizado; não justificam violar a constituição

## 1. North Star candidata

`weekly_minds_changed`: número de participantes distintos que registraram ao menos uma mudança válida de posição nos últimos sete dias.

Ela é candidata, não dogma. Isoladamente pode premiar afirmações ambíguas, alternância artificial ou polarização. Deve ser lida com os guardrails abaixo.

## 2. Métricas do funil

- `visitor_to_position_rate`: visitantes elegíveis de uma Arena que fazem escolha local antes de ver o agregado.
- `position_to_signup_rate`: visitantes que, após escolha local, concluem cadastro e confirmam posição.
- `signup_to_argument_rate`: novas contas que publicam primeiro argumento dentro da janela definida.
- `argument_to_mind_changed_rate`: argumentos válidos que recebem ao menos uma atribuição válida dentro da janela.
- `free_ink_exhaustion_rate`: contas ativas que consomem toda a franquia no período.
- `ink_purchase_conversion`: contas elegíveis que compram INK.
- `arena_purchase_conversion`: contas elegíveis que compram Arena Pass.
- `member_conversion`: contas elegíveis que iniciam Member pago.

Cada taxa precisa declarar denominador, janela, elegibilidade e exclusões. “Conta criada” não deve ser substituída por “pessoa”.

## 3. Retenção

- `7_day_retention`: participante retorna e realiza ação significativa entre o 7º e o 13º dia após ativação.
- `30_day_retention`: participante retorna e realiza ação significativa entre o 30º e o 44º dia.

Ação significativa é ler argumentos de mais de uma posição, publicar, responder, confirmar posição, mudar ou atribuir — não apenas abrir email ou página.

## 4. Qualidade e saúde

Ler sempre junto da North Star:

- proporção de participantes que leem mais de uma posição;
- mudanças com ao menos uma atribuição;
- concentração de atribuições por autor e por Arena;
- reversões repetidas pela mesma conta;
- denúncias por cem contribuições;
- violações confirmadas por cem contribuições;
- atribuições e posições invalidadas por abuso;
- participantes que relatam ter encontrado argumento novo;
- tempo mediano até primeira contribuição útil;
- diversidade de Arenas que geram mudanças.

## 5. Negócio

- receita bruta e líquida por produto;
- MRR e churn de Member;
- margem de contribuição;
- custo de pagamento, infraestrutura, suporte e moderação por conta ativa;
- uso e passivo operacional de INK comprado;
- reembolsos, chargebacks e fraude.

Receita nunca entra em ranking de conteúdo.

## 6. Eventos analíticos mínimos

Eventos devem descrever ações, sem texto de conteúdo nem dados pessoais:

- Arena visualizada;
- escolha local registrada;
- agregado revelado;
- posição confirmada;
- argumento visualizado;
- argumento publicado;
- posição alterada;
- atribuição concluída ou pulada;
- denúncia iniciada e enviada;
- checkout iniciado e confirmado.

Não coletar tudo “para usar depois”. Cada evento deve ter dono, pergunta de negócio e prazo de retenção.

## 7. Antimétricas

Não otimizar como objetivo primário:

- tempo de tela;
- número bruto de comentários;
- notificações abertas;
- controvérsia ou raiva;
- número de mudanças sem validação de qualidade;
- receita por participante isoladamente.

Essas métricas podem indicar operação, mas criam incentivos contrários ao propósito quando viram objetivo.
