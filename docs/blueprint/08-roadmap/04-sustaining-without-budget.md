# 08.4: Keeping it going without a budget

Diffluence has no paying customers and no budget; it asks for donations. Blueprint 08 plans years of work. This chapter looks at what research says about funding and sustaining open-source work, what routes exist for a project like this one, and what that means for the roadmap.

## 4.1 What the research says

- **Donations rarely pay for a person.** Overney, Meinicke, Kästner and Vasilescu (ICSE 2020) found 25,885 GitHub projects asking for donations, mostly to pay for engineering. Many receive some, but the amounts rarely fund a full-time position, and they found no clear evidence that donations change how active a project is.
- **Infrastructure is undervalued.** Eghbal's report *Roads and Bridges* (Ford Foundation, 2016) describes how widely used open-source software runs on the unpaid time of a few people, and how that makes it fragile. Her later book *Working in Public* (2020) adds that attention from users, not code, is the scarce resource a maintainer must manage.
- **What predicts that a project keeps going.** Valiev, Vasilescu and Herbsleb studied the PyPI ecosystem (ESEC/FSE 2018): projects with more contributors, and those used by others, are more likely to stay active. Coelho and Valente's survey of failed projects (ESEC/FSE 2017) found lack of time of the main developer among the top causes. The truck factor of most popular projects is one or two people (Avelino et al., ICPC 2016).

The plain conclusion: a project of one maintainer must plan for pauses, make it easy for others to join, and treat money as a bonus, not a plan.

## 4.2 Routes that exist

| Route | What it gives | What it asks |
|---|---|---|
| **GitHub Sponsors, Open Collective** | Small recurring donations, shown publicly | A payout account; Open Collective works through a fiscal host that holds the money |
| **NLnet and the EU's Next Generation Internet programmes** | Grants for open internet software, paid per milestone. The NGI Zero Commons Fund held its thirteenth and final call on 1 June 2026; Horizon Europe's Open Internet Stack calls follow on | A written proposal per milestone; a person or organisation that signs and receives the money |
| **Sovereign Tech Agency** (Germany) | Investment in foundational open-source components, a fellowship for maintainers, and support for standards work | Applications in rounds; aimed at components many others depend on |
| **A foundation or fiscal host** | Holds money and trademarks, outlives one person | Membership terms; a legal relationship |
| **Paid support or hosting by others** | Companies may sell support for an Apache-2.0 product without asking | Nothing from the project, and no income for it either |

**The EU Cyber Resilience Act.** Recital 15 of Regulation (EU) 2024/2847 says that accepting donations without the intention of making a profit is not a commercial activity, so a project that only takes donations is not a "manufacturer" under the Act. An "open-source software steward", an organisation that supports such software for commercial use, has lighter duties: a cybersecurity policy and cooperation on vulnerabilities. Diffluence's disclosure process and security policy already cover much of that (blueprint 06, SEC-1041).

## 4.3 What that means for the roadmap

- Every phase must leave the product working and maintainable if work stops after it (chapter 1, RMP-004).
- The cost of running the project itself must stay near zero: CI on free tiers, no paid services, no infrastructure that needs a person on call. Release signing with Sigstore and GitHub attestations costs nothing (07.5).
- The best investment in the long run is making it easy to contribute: a clear `CONTRIBUTING.md`, small first issues, and the specs and reports already in the repository.

## 4.4 Recommendations

- **RMP-030** · new · **Medium.** Turn on GitHub Sponsors or an Open Collective page only when a payout route exists that fits the product owner's wish to stay anonymous; until then, the README says how to help with time instead of money.
- **RMP-031** · new · **Medium.** Prepare one grant proposal per large theme that fits public-interest funding (self-hosted knowledge for public bodies and small organisations, document control for regulated work, accessibility), written as milestones from the specs that exist, so it is ready when a call opens.
- **RMP-032** · new · **Medium.** Keep the cost of the project's own infrastructure at zero, and write down every service it depends on, with what happens if that service ends.
- **RMP-033** · new · **Low.** Label good first issues from the open lines of the blueprints (Low lines are ideal), and keep the way of working in `CONTRIBUTING.md` short enough to read in ten minutes.

## Sources

- Overney, Meinicke, Kästner, Vasilescu: [How to Not Get Rich: An Empirical Study of Donations in Open Source](https://doi.org/10.1145/3377811.3380410), ICSE 2020.
- Eghbal: [Roads and Bridges: The Unseen Labor Behind Our Digital Infrastructure](https://www.fordfoundation.org/work/learning/research-reports/roads-and-bridges-the-unseen-labor-behind-our-digital-infrastructure/), Ford Foundation, 2016.
- Eghbal: Working in Public: The Making and Maintenance of Open Source Software, Stripe Press, 2020.
- Valiev, Vasilescu, Herbsleb: [Ecosystem-Level Determinants of Sustained Activity in Open-Source Projects: A Case Study of the PyPI Ecosystem](https://doi.org/10.1145/3236024.3236062), ESEC/FSE 2018.
- Coelho, Valente: [Why Modern Open Source Projects Fail](https://doi.org/10.1145/3106237.3106246), ESEC/FSE 2017.
- Avelino, Passos, Hora, Valente: [A Novel Approach for Estimating Truck Factors](https://doi.org/10.1109/ICPC.2016.7503718), ICPC 2016.
- NLnet: [NGI Zero Commons Fund](https://nlnet.nl/commonsfund/).
- Sovereign Tech Agency: [Programs](https://www.sovereign.tech/programs).
- [Regulation (EU) 2024/2847, the Cyber Resilience Act](https://eur-lex.europa.eu/eli/reg/2024/2847/oj), recital 15 and article 24; European Commission: [CRA and open source](https://digital-strategy.ec.europa.eu/en/policies/cra-open-source).
- [GitHub Sponsors](https://github.com/sponsors), [Open Collective, fiscal hosts](https://docs.opencollective.com/help/fiscal-hosts/fiscal-hosts).

All sources of this study are also in [sources.md](sources.md).
