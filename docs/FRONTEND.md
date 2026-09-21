# Arquitetura do frontend

**Status:** padrão obrigatório

**Runtime de terceiros no browser:** nenhum

## 1. Princípios

1. A plataforma do browser é o framework.
2. HTML funciona antes do aprimoramento quando a jornada permitir.
3. Um componente conhece seu contrato, não a página inteira.
4. Estado remoto pertence ao servidor; não criar cópia global sem necessidade.
5. Acessibilidade e segurança são parte da API do componente.
6. Performance é controlada por orçamento, não por impressão.

## 2. Toolchain

TypeScript 7 é compilado pelo `tsc` oficial para módulos ESM. O frontend não possui bundler, JSX, transpilador adicional, preprocessor CSS ou runtime de framework.

Imports relativos usam a extensão `.js` que existirá após emissão. Bare specifiers de pacotes são proibidos. O compilador preserva uma saída previsível de módulos; CSS é carregado por `<link>` ou pela composição documentada da página, não por plugin de bundler.

O `tsconfig.json` futuro deve habilitar, no mínimo:

- `strict`;
- `noUncheckedIndexedAccess`;
- `exactOptionalPropertyTypes`;
- `useUnknownInCatchVariables`;
- `noImplicitOverride`;
- `noFallthroughCasesInSwitch`;
- `verbatimModuleSyntax`;
- `isolatedDeclarations` quando a estrutura permitir.

`any` exige comentário com motivo e issue de remoção quando não for boundary inevitável. Preferir `unknown` e narrowing.

## 3. Tipos de componentes

- **Primitive:** botão, dialog, tabs e feedback; não conhece domínio.
- **Domain component:** apresenta ou coleta um conceito como argumento ou posição.
- **Page controller:** compõe componentes, URL, carregamento e navegação.
- **Service:** client HTTP, i18n, sessão visível e telemetria; expõe interface pequena.

Componentes não buscam service locator global. Dependências entram pelo construtor/factory, propriedade ou contexto explícito da página.

## 4. Contrato de componente

Todo componente documenta:

- tag e responsabilidade;
- atributos e propriedades;
- eventos emitidos e seus payloads;
- estados visual, vazio, carregando, erro e desabilitado;
- comportamento de teclado e foco;
- requisitos de CSS e tokens;
- efeitos externos e cancelamento;
- exemplos de uso.

Custom Elements são registrados uma vez. O componente trata reconexão ao DOM sem duplicar listeners. `disconnectedCallback` cancela requests e timers próprios.

## 5. Comunicação

- pai passa dados; filho emite intenção;
- eventos sobem com `CustomEvent` tipado;
- componentes irmãos não se importam nem se procuram no DOM;
- page controller coordena jornadas;
- alterações de URL usam APIs nativas e permanecem compartilháveis;
- nenhum event bus global genérico.

## 6. Client HTTP

Um único núcleo implementa:

- base URL e versão da API;
- headers comuns e request ID;
- CSRF;
- timeout e cancelamento;
- parsing de Problem Details;
- política de retry apenas para operações seguras ou idempotentes;
- tratamento uniforme de sessão expirada;
- observabilidade sem payload privado.

Clients por domínio compõem esse núcleo e retornam contratos tipados. Componentes nunca chamam `fetch` diretamente.

Tipos de transporte gerados ficam em `web/src/contracts/generated.ts`. O arquivo é sobrescrito pelo gerador interno e nunca recebe edição manual ou comportamento.

## 7. Estado

- estado efêmero permanece no componente;
- estado de página permanece no page controller;
- URL representa filtros e paginação compartilháveis;
- servidor é autoridade para conta, wallet, posição e permissões;
- optimistic UI só é usada quando rollback é claro;
- nenhuma mutação crítica é considerada concluída antes da resposta do servidor.

## 8. CSS nativo

Tokens cobrem cor, tipografia, espaçamento, raio, sombra, duração e z-index. Tokens semânticos substituem valores de marca dentro dos componentes.

Cada componente possui folha local importada pelo entrypoint da página. `@layer` define precedência. Container queries substituem breakpoints baseados na tela quando o componente depende de seu contêiner.

CSS deve funcionar em forced colors, zoom de 200%, reduced motion e navegação por teclado. Não esconder foco sem alternativa visível.

## 9. Segurança de renderização

- texto de usuário é atribuído por `textContent` ou criado como text node;
- `innerHTML`, `outerHTML`, `insertAdjacentHTML` e `eval` são proibidos para dados dinâmicos;
- JSON inicial é serializado com escaping seguro pelo servidor;
- URLs passam por parser e allowlist de protocolo;
- atributos de evento inline são proibidos;
- CSP bloqueia script não autorizado.

## 10. i18n

Textos não ficam espalhados em componentes. Catálogos tipados por locale vivem em módulo próprio. Arena mantém idioma independente do locale da interface. Formatação usa `Intl` nativo.

## 11. Orçamentos iniciais

Orçamentos são guardrails a validar:

- JavaScript inicial por página pública: alvo máximo de 50 KB comprimidos;
- CSS inicial: alvo máximo de 40 KB comprimidos;
- nenhuma dependência remota bloqueando render;
- zero layout shift introduzido por componente sem reserva de espaço;
- interações principais acompanhadas por Core Web Vitals reais.

Exceções exigem medida, justificativa e revisão.

### 11.1 O gate que mede

`make audit-web` (`tools/webaudit`) mede o build entregue em `web/dist` e falha fechado. Cinco perguntas, cada uma com o artefato que interroga:

1. **orçamentos** — a soma dos bytes comprimidos de cada módulo do fecho de uma página pública (`web/dist/pages/*.js` e tudo que ele importa) contra os 50 KB, e o total das folhas de estilo do build contra os 40 KB. A medida é por resposta, não por concatenação: o browser busca um módulo por request, cada um com seu próprio fluxo de compressão, então o que se soma é o que o visitante baixa — nunca o peso de um bundle, que não existe neste pipeline. A compressão é gzip: brotli exigiria uma dependência que a política de dependências não admite, e o orçamento é guarda de regressão, não promessa sobre o que a borda negocia. "KB" é lido como 1.000 bytes, a leitura mais restrita.
2. **specifiers** — todo import do grafo entregue é relativo, resolve para um arquivo que o manifest publica e fica dentro do build: nada de origem externa (`https://…`, `//…`), de caminho absoluto, de bare specifier (o browser não tem resolver nem diretório de pacotes) nem de caminho não publicado.
3. **csp** — o código entregue não carrega `eval`, `new Function`, `document.write`, `javascript:` nem sink de HTML (`innerHTML`, `outerHTML`, `insertAdjacentHTML`), e a política que o binário entrega (lida do próprio `internal/platform/securityheaders`) continua sem `'unsafe-inline'`, sem `'unsafe-eval'` e sem origem alguma. Os comentários são apagados antes da leitura — este repositório documenta as regras nos módulos que as obedecem, e o gate não pode reprovar a própria documentação — preservando as linhas do arquivo original, para que a violação aponte a linha certa.
4. **network** — `fetch`, `XMLHttpRequest`, `WebSocket`, `EventSource` e `sendBeacon` aparecem somente sob `web/src/core/`: é o core que carrega timeout, CSRF, identidade de request e Problem Details, e uma página que busca por conta própria ignora todas essas garantias.
5. **build** — o manifest existe, todo arquivo que ele declara existe e há ao menos um entry em `pages/`. Um gate que não consegue ver o que mede não prova nada: arquivo declarado e ausente é erro, não skip.

O digest de cada registro do manifest é o contrato do servidor (`internal/platform/assets`, endereço imutável e `ETag`) e é verificado onde é usado; este gate mede conteúdo, não identidade.

O gate roda no `make verify`; os testes de `tools/webaudit` provam cada regra sobre um build de fixture, de modo que uma sonda que quebra uma regra falha nomeando o módulo e a linha, e o build real volta a passar quando ela sai.

## 12. Testes

- unidades para funções puras e state machines;
- contract tests do client HTTP;
- testes em browser real para lifecycle de Custom Elements;
- E2E para jornadas críticas;
- auditoria do build entregue para orçamentos e dependências (`make audit-web`);
- axe ou auditoria equivalente no CI sem virar dependência de runtime;
- screenshots para regressões de layout em páginas essenciais;
- teste com JavaScript desabilitado para fluxos progressivos prometidos.

## 13. Definição de pronto

Um componente só está pronto quando possui contrato, tipos estritos, teclado, estados completos, cancelamento de efeitos, CSS isolado, testes relevantes e nenhum dado privado em logs ou DOM público.
