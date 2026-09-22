# Goyim Arena

Goyim Arena é uma plataforma pública de debates estruturados que mede mudanças de opinião e os argumentos que contribuíram para elas.

O projeto está na fase de definição do produto e da arquitetura. Ainda não há implementação. A documentação versionada é, por enquanto, a fonte de verdade do projeto.

## Comece por aqui

- [Índice da documentação](docs/README.md)
- [Visão do produto](docs/PRODUCT.md)
- [Constituição](docs/CONSTITUTION.md)
- [Regras de negócio](docs/BUSINESS_RULES.md)
- [Definição do MVP](docs/MVP.md)
- [Hipóteses, riscos e validação](docs/RISKS_AND_ASSUMPTIONS.md)
- [Stack tecnológica](docs/STACK.md)
- [Arquitetura](docs/ARCHITECTURE.md)
- [Como contribuir](CONTRIBUTING.md)

## Estado atual

**Fase:** descoberta e documentação

**Nome:** Goyim Arena

**Mercados iniciais:** Brasil e internacional, em português do Brasil e inglês dos Estados Unidos — decisão do titular registrada em [docs/GOVERNANCE.md](docs/GOVERNANCE.md) (`launch-markets`), que também registra o que ainda bloqueia o beta

**Stack aprovada:** TypeScript 7 e CSS nativo no frontend; Go e PostgreSQL no backend; Caddy e Cloudflare na operação

**Próximo passo:** validar o problema e os fluxos centrais antes de implementar o SaaS completo

## Como contribuir nesta fase

Abra uma discussão ou proposta contendo:

1. o problema observado;
2. a mudança sugerida;
3. o efeito esperado;
4. os riscos ou trade-offs;
5. como a hipótese poderia ser testada.

Decisões de produto devem ser registradas no [log de decisões](docs/DECISIONS.md), e decisões técnicas nos [ADRs](docs/adr/README.md).
