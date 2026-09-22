# Regras de negócio

**Status:** decisões propostas para o MVP

**Versão:** 0.1

Este documento define comportamento do produto, independentemente da implementação técnica.

## 1. Atores

- **Visitante:** não autenticado; lê conteúdo público e faz uma escolha local para liberar o agregado.
- **Participante:** conta com email verificado; registra posições, publica e atribui influência.
- **Criador:** participante que publica uma Arena consumindo um Arena Pass.
- **Moderador:** analisa denúncias e aplica ações segundo política pública.
- **Administrador:** executa ações operacionais restritas e auditadas.

Pagar não cria um papel com mais influência editorial.

## 2. Arena

Uma Arena contém uma afirmação principal, contexto opcional, categoria, idioma, autoria, data de publicação, encerramento opcional e estado.

### 2.1 Qualidade mínima da afirmação

Para ser publicada, a afirmação deve:

- expressar uma proposição com a qual seja possível concordar ou discordar;
- ser compreensível sem depender de uma enquete escondida no contexto;
- evitar reunir várias proposições independentes;
- informar idioma e categoria;
- não violar a política de conteúdo.

Perguntas devem ser reformuladas como afirmações. Exemplo: “A AGI existirá até 2040” em vez de “Quando teremos AGI?”.

### 2.2 Estados

- `draft`: visível apenas ao criador.
- `published`: aberta à participação.
- `closed`: somente leitura; não aceita novas posições, argumentos ou mudanças.
- `restricted`: permanece acessível conforme decisão de moderação, mas alguma interação foi limitada.
- `removed`: indisponível ao público por decisão registrada.

O Arena Pass é consumido apenas na transição bem-sucedida de `draft` para `published`.

### 2.3 Imutabilidade e correções

A afirmação principal e seu idioma não são editáveis depois da publicação. O contexto pode receber correções futuras apenas com revisão pública, data e autor. No MVP, a opção simples é não permitir edição pós-publicação: o criador pode fechar a Arena e publicar outra, mantendo ambas ligadas por uma nota.

Slug é endereço amigável, não identidade permanente. Links e exportações devem usar também um identificador estável.

## 3. Posições

Valores permitidos:

- `agree` — concordo;
- `disagree` — discordo;
- `undecided` — indeciso.

### 3.1 Visitante versus posição oficial

A escolha de um visitante serve apenas para liberar a visualização do agregado naquela sessão. Ela não é contabilizada, não cria reputação e pode desaparecer.

Uma posição entra no agregado oficial somente após ser confirmada por uma conta elegível com email verificado. Ao se cadastrar, a pessoa deve confirmar novamente a escolha; a plataforma não deve convertê-la silenciosamente.

### 3.2 Posição inicial

- Cada participante tem no máximo uma posição inicial por Arena.
- A primeira posição confirmada é imutável como fato histórico.
- A posição atual começa igual à inicial e muda apenas por eventos posteriores.
- Resultados agregados aparecem somente depois de uma escolha local ou posição confirmada.
- Compartilhamentos, prévias e metadados públicos não devem revelar o agregado antes dessa escolha.
- Posição individual e histórico de mudança são privados por padrão; o público vê agregados.
- Argumentos continuam públicos e podem, por seu conteúdo, revelar a posição que o autor decidiu expressar.

### 3.3 Mudança de posição

Cada mudança registra posição anterior, nova posição e momento. Não é permitido registrar mudança para o mesmo valor atual.

Uma pessoa pode mudar mais de uma vez, mas o histórico permanece acessível a ela e aos controles autorizados. Arenas fechadas não aceitam novas mudanças.

## 4. Argumentos e respostas

Um argumento:

- pertence a uma única Arena;
- possui um autor;
- declara sua relação com a afirmação: a favor, contra ou contextual/neutra;
- pode responder diretamente a um argumento da mesma Arena;
- contém no máximo 3.000 grapheme clusters;
- pode ter fontes estruturadas;
- possui data e identidade estável.

Respostas são argumentos e obedecem às mesmas regras de custo e moderação.

### 4.1 Conteúdo e edição

No MVP, argumentos publicados são imutáveis. Antes de publicar, a interface deve oferecer prévia clara e custo em INK. O autor pode retirar o argumento da exibição, mas isso não apaga o fato histórico nem gera reembolso automático.

Uma evolução futura pode permitir revisões auditáveis. Edição silenciosa nunca será permitida.

### 4.2 Fontes

Fontes apoiam uma alegação, mas não recebem selo automático de verdade. Cada fonte pode ter URL e descrição curta. A plataforma pode bloquear URLs maliciosas ou indisponibilizar links sem alterar silenciosamente o texto do argumento.

### 4.3 Ordenação inicial

No MVP, cada relação com a afirmação tem sua própria lista, ordenada por publicação recente ou antiga conforme escolha explícita do usuário. “Minds changed” pode ser exibido como dado e filtro, mas não deve ser o ranking padrão, para evitar vantagem cumulativa precoce.

Qualquer nova fórmula de ranking exige regra pública e histórico.

## 5. Mudança e atribuição de influência

Depois de registrar uma mudança, o participante pode selecionar de zero a três argumentos que contribuíram para ela.

Um argumento elegível deve:

- pertencer à mesma Arena;
- ter sido publicado antes da mudança;
- ser de outro autor;
- estar disponível ou ter estado disponível à pessoa antes da atribuição;
- não estar invalidado por fraude ou moderação.

A relação do argumento com a afirmação não limita sua elegibilidade: um argumento do lado anterior também pode revelar uma fragilidade e contribuir para a mudança.

### 5.1 Integridade da atribuição

- Não é possível atribuir influência ao próprio argumento.
- O mesmo argumento aparece no máximo uma vez por evento de mudança.
- Atribuições são fatos históricos; não são vendidas nem transferidas.
- A identidade de quem atribuiu é privada por padrão; autores e público veem contagens válidas, não a lista de pessoas.
- A plataforma pode invalidar atribuições fraudulentas sem apagar o registro administrativo da decisão.
- A tela deve dizer “contribuiu para minha mudança”, não “causou minha mudança”.

## 6. Reputação pública

Não existe score universal. O perfil pode mostrar:

- argumentos publicados;
- respostas publicadas;
- fontes adicionadas;
- pessoas distintas que atribuíram uma mudança a pelo menos um argumento do autor;
- atribuições válidas recebidas;
- distribuição desses fatos por categoria e idioma.

Para reduzir inflação por alternância repetida, a manchete “pessoas influenciadas” conta cada participante no máximo uma vez por autor em cada Arena. Contagens detalhadas podem mostrar eventos adicionais separadamente.

Métricas removidas por fraude ou decisão de moderação não integram os totais válidos. A existência da correção deve constar nos agregados de transparência sem identificar a pessoa.

## 7. Elegibilidade e integridade dos agregados

No MVP, integram resultados oficiais apenas contas:

- com email verificado;
- não suspensas no momento do evento;
- que não tenham sido identificadas como duplicação ou automação abusiva;
- cujos eventos não tenham sido invalidados por revisão.

Os resultados devem exibir quantidade de participantes, momento da atualização e aviso de que não representam a população geral.

## 8. INK e publicação

- Um grapheme cluster do conteúdo do argumento custa 1 INK.
- Espaços, pontuação e quebras de linha também contam quando são grapheme clusters.
- URLs e descrições de fontes não consomem INK no MVP, mas têm limites próprios contra abuso.
- O custo é mostrado antes da confirmação.
- Publicação e consumo devem formar uma única operação: ou ambos acontecem, ou nenhum acontece.
- Saldo incluído no plano é consumido antes do saldo comprado.
- Falha técnica não pode consumir INK sem publicar; correções operacionais devem ser auditadas.

## 9. Arena Pass

- Criar e editar rascunho não consome passe.
- Publicar uma nova Arena consome exatamente um passe.
- Fechar, restringir ou remover uma Arena não devolve passe automaticamente.
- Remoção causada por erro da plataforma ou moderação revertida pode gerar restituição auditada.
- Passes comprados e passes incluídos em assinatura devem permanecer distinguíveis.

## 10. Encerramento, remoção e conta

- Encerrar uma Arena preserva sua leitura e exportação pública.
- Remover conteúdo da exibição não deve quebrar silenciosamente contagens históricas; a interface usa marcadores apropriados.
- Ao excluir a conta, dados privados são eliminados ou anonimizados conforme obrigação e necessidade; conteúdo público pode ser preservado sob autor anonimizado quando necessário à integridade da conversa, conforme aviso prévio e política de privacidade.
- Banimento não transfere autoria nem reputação.

## 11. Invariantes

Estas regras não podem ser violadas por nenhum fluxo:

1. dinheiro não altera peso de posição;
2. visitante não entra no agregado oficial;
3. posição inicial confirmada não é reescrita;
4. toda posição atual deriva do histórico de mudanças;
5. ninguém atribui influência a si próprio;
6. no máximo três atribuições por mudança;
7. argumento e débito de INK são indivisíveis;
8. um Arena Pass só é consumido por publicação bem-sucedida;
9. remoção pública não apaga trilha administrativa;
10. toda métrica de reputação pode ser recalculada a partir de fatos válidos.

## 12. Questões ainda abertas

As decisões humanas do lançamento — idade mínima, licença, contato de privacidade, retenção, mercados, termos e canal de segurança — vivem em [GOVERNANCE.md](GOVERNANCE.md), e não em cópias espalhadas por este e pelos demais documentos.

- ~~Existe idade mínima geral ou por mercado?~~ **Respondida:** dezoito anos em todos os mercados, sem exceção por mercado e sem fluxo para menores (`GOVERNANCE.md`, `age-minimum`).
- Certas categorias exigirão aviso, limitação etária ou revisão prévia?
- Quando uma Arena pode ser duplicada versus considerada continuação?
- Qual limite de fontes por argumento equilibra utilidade e spam?
- Posições invalidadas por abuso continuam visíveis ao próprio usuário?
- Haverá janela de arrependimento antes de uma mudança ser confirmada e entrar nos agregados?
