# Padrão de commits

O projeto usa [Conventional Commits 1.0.0](https://www.conventionalcommits.org/pt-br/v1.0.0/).

## Formato

```text
type(scope)!: descrição curta

corpo opcional explicando contexto e motivação

rodapé opcional
```

O `!` é usado somente para mudança incompatível e precisa de `BREAKING CHANGE:` no rodapé.

## Tipos permitidos

- `feat`: nova capacidade percebida pelo usuário ou integrador.
- `fix`: correção de comportamento defeituoso.
- `docs`: documentação apenas.
- `refactor`: mudança interna sem alterar comportamento pretendido.
- `perf`: melhoria de desempenho.
- `test`: criação ou correção de testes.
- `build`: build, imagens, módulos ou empacotamento.
- `ci`: workflows e automação de integração/deploy.
- `chore`: manutenção que não cabe nos tipos anteriores.
- `revert`: reversão explícita de commit anterior.

## Scopes recomendados

- `identity`, `profiles`, `arenas`, `positions`;
- `arguments`, `persuasion`, `wallet`, `billing`;
- `moderation`, `transparency`, `audit`, `jobs`;
- `web`, `db`, `infra`, `deps`, `security`, `docs`, `project`.

Adicionar um scope exige que ele represente uma área duradoura. Omitir é aceitável quando a mudança cruza várias áreas de maneira indivisível.

## Regras

- descrição no imperativo, minúscula e sem ponto final;
- primeira linha com no máximo 72 caracteres sempre que possível;
- um commit deve representar uma mudança lógica reversível;
- não misturar refactor amplo com mudança funcional;
- explicar o “porquê” no corpo quando o diff não for suficiente;
- referenciar issue no rodapé, por exemplo `Closes #123`;
- nunca inserir segredo, dado pessoal ou informação sensível na mensagem;
- commits de código gerado devem acompanhar a fonte que os produziu.

Mensagens podem ser em português, mantendo `type` e `scope` padronizados em inglês.

## Exemplos

```text
feat(wallet): debita ink de forma atômica ao publicar argumento
fix(billing): ignora webhook stripe já processado
docs(architecture): registra estratégia de cache público
test(positions): cobre mudança concorrente de posição
ci(security): adiciona varredura de segredos
```

Mudança incompatível:

```text
feat(export)!: publica schema v2 da exportação de arena

BREAKING CHANGE: consumidores devem ler position_aggregates em vez de positions.
```

## Branches e pull requests

Branches usam uma categoria e descrição curta, por exemplo:

- `feat/wallet-ledger`;
- `fix/webhook-idempotency`;
- `docs/moderation-policy`;
- `chore/dependency-updates`.

O título do pull request segue o mesmo padrão de commit. Ao usar squash merge, esse título se torna a mensagem final.

## Versionamento

Quando houver releases:

- `fix` gera patch;
- `feat` gera minor;
- `BREAKING CHANGE` gera major;
- `docs`, `test`, `ci`, `build`, `refactor`, `perf` e `chore` não geram versão por si, salvo política futura explícita.

## Configuração local

O repositório inclui `.gitmessage`. Ative o template com:

```bash
git config commit.template .gitmessage
```

Validação automática de título de PR e commits será adicionada junto do primeiro pipeline, fixando actions de terceiros por SHA.
