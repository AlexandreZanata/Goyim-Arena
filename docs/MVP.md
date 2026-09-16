# MVP

**Status:** escopo recomendado

**Objetivo:** validar o loop de mudança de opinião, não lançar uma rede social completa

## 1. Pergunta do MVP

> Pessoas encontram valor suficiente em registrar uma posição, explorar argumentos estruturados e atribuir sua mudança a argumentos específicos para voltar e participar novamente?

Monetização deve ser validada, mas não é a primeira prova. Se a mudança e a atribuição não gerarem valor, otimizar compras de INK não salvará o produto.

## 2. Estratégia recomendada em duas etapas

### Etapa A — piloto concierge

Antes do SaaS completo, conduzir entre 5 e 10 Arenas com 30 a 100 participantes convidados. Criação de Arena, seleção de temas e moderação podem ser manuais. Pagamento real, assinatura e automações completas não são necessários.

O piloto deve testar:

- compreensão da afirmação;
- resistência ao registro prévio da posição;
- leitura de ambos os lados;
- frequência e qualidade das mudanças;
- disposição de atribuir influência;
- problemas de abuso e moderação.

### Etapa B — MVP público controlado

Somente depois de sinais positivos, disponibilizar autoatendimento, wallet, compras e assinatura para uma comunidade ainda limitada.

## 3. Escopo funcional do MVP público

### Conta e perfil

- cadastro e login;
- verificação de email;
- username público;
- locale;
- perfil com métricas factuais;
- exportação e solicitação de exclusão.

### Descoberta

- feed básico por idioma;
- página pública indexável de Arena;
- busca simples por título/afirmação pode ficar fora se o feed for suficiente no lançamento;
- filtros de categoria e status.

### Arena

- rascunho e publicação com Arena Pass;
- afirmação, contexto, categoria, idioma e encerramento opcional;
- estado publicado/fechado/restrito/removido;
- declaração imutável após publicação;
- exportação pública versionada.

### Posição e resultado

- escolha local de visitante para revelar agregado;
- confirmação autenticada da posição inicial;
- resultados com tamanho da amostra e aviso metodológico;
- posição e mudanças individuais privadas por padrão;
- mudanças imutáveis de posição;
- histórico pessoal e agregado.

### Argumentação

- argumento a favor, contra ou contextual;
- respostas em um nível lógico recursivo, com interface inicial limitada para legibilidade;
- fontes por URL e descrição;
- limite de 3.000 grapheme clusters;
- prévia e custo antes de publicar;
- retirada pelo autor sem edição silenciosa.

### Persuasão

- convite à atribuição após mudança;
- zero a três argumentos elegíveis;
- bloqueio de autoatribuição;
- métricas por argumento, autor, Arena e categoria;
- correção de atribuições fraudulentas via moderação.

### Capacidade e cobrança

- franquia Free;
- saldo comprado de INK;
- Arena Pass comprado;
- Member mensal;
- histórico compreensível de créditos e consumos;
- confirmação de compra e suporte a reembolso.

### Confiança

- denúncia;
- fila e decisão de moderação;
- recurso;
- suspensão e restrição;
- registro de ações administrativas;
- páginas de constituição, moderação, transparência e roadmap.

### Idiomas

- interface pt-BR e en-US;
- idioma fixado por Arena;
- feeds separados por idioma;
- nenhuma tradução automática no MVP.

## 4. Fora do MVP

- mensagens diretas e chat;
- seguidores;
- curtidas e reações genéricas;
- áudio, vídeo e debates ao vivo;
- aplicativo móvel nativo;
- IA para julgar, ranquear ou resumir;
- tradução automática;
- blockchain, token, mercado ou apostas;
- pagamento a autores;
- ranking personalizado ou secreto;
- múltiplos planos pagos;
- organizações e equipes;
- API pública geral além da exportação de debate;
- identidade governamental ou prova de humanidade forte.

## 5. Jornadas críticas

### Visitante entende uma Arena

1. Abre uma URL pública e lê afirmação e contexto.
2. Escolhe concordo, discordo ou indeciso.
3. Vê o agregado marcado como “participantes verificados da plataforma”.
4. Compara argumentos dos diferentes lados.
5. É convidado a criar conta apenas quando tenta participar de forma persistente.

### Participante publica argumento

1. Escolhe relação com a afirmação e escreve.
2. Vê limite, custo e saldo que será usado.
3. Revisa conteúdo e fontes.
4. Confirma publicação.
5. Conteúdo e débito aparecem juntos; falha não produz estado parcial.

### Participante muda de posição

1. Seleciona nova posição.
2. Confirma que a mudança será histórica e usada em agregados, mas que sua posição individual permanecerá privada por padrão.
3. Evento é registrado.
4. Seleciona até três argumentos de outros autores ou pula.
5. Métricas válidas são atualizadas sem revelar quem mudou ou atribuiu.

### Criador publica Arena

1. Cria e revisa rascunho.
2. Confirma que a afirmação ficará imutável.
3. Vê o passe que será consumido.
4. Publica.
5. Recebe URL estável e ferramentas de compartilhamento.

## 6. Critérios de saída do piloto

Avançar para o MVP público apenas se houver, como referência inicial:

- pelo menos 60% dos participantes entendendo o fluxo sem explicação individual;
- pelo menos 30% lendo argumentos de mais de uma posição;
- mudanças de opinião genuínas em múltiplas Arenas, não concentradas em um único tema;
- ao menos metade das mudanças contendo uma atribuição ou explicação de por que não houve;
- intenção qualitativa clara de retornar para outra Arena;
- carga de moderação compatível com operação manual inicial.

Esses limiares são critérios de aprendizado, não metas para maquiar comportamento.

## 7. Critérios de sucesso do MVP público

Após uma coorte de uso suficiente, procurar:

- retenção motivada por novas Arenas e respostas, não por notificações agressivas;
- crescimento de `weekly_minds_changed` com diversidade de autores e Arenas;
- taxa estável de denúncias e baixa incidência de manipulação confirmada;
- participantes relatando que encontraram argumentos que não conheciam;
- alguma disposição de pagar por capacidade sem piora observável da igualdade de alcance.

## 8. Condições para interromper ou reformular

Reconsiderar o produto se:

- mudança de opinião for rara demais para sustentar a proposta;
- atribuições forem majoritariamente estratégicas ou recíprocas;
- o bloqueio pré-agregado gerar abandono sem benefício perceptível;
- INK silenciar participação legítima mais do que reduz spam;
- moderação necessária for incompatível com a capacidade da equipe;
- usuários interpretarem os resultados como representativos apesar dos avisos.
