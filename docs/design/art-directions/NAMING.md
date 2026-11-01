# Naming

Screening record for the product name. Nothing here is legal clearance — it is
a search of registries, package indexes and the web. Trademark status needs a
proper USPTO search (TESS is retired and its replacement is JavaScript-only)
and, if this ships commercially, a lawyer.

Checked 2026-11-01. Domains via RDAP, packages via the npm and PyPI registries,
org names via the GitHub API.

## Why the short words keep failing

Every short, warm, common English noun is taken, and the warmer the word the
more likely a company owns it. Two mechanical facts recur:

- Every `.com` of five letters or fewer was registered in the 1990s.
- Every short English noun is already a package on npm or PyPI, usually both.

Names that survive are either **longer than the available words** (Germinal,
Trilobate) or **made up**. Testing dictionary words one at a time has a hit rate
of roughly zero; the table below is the evidence.

## Dead

| Name | Why | Evidence |
|---|---|---|
| **Guild** | Two live companies, one per market | Guild.ai ("control plane for AI agents", $44M raised, guild.ai 2017); Guild.xyz (community management platform, guild.xyz 2016); Discord's API calls a server a "guild" |
| **Gravity** | A live AI-agents company plus Google's agent IDE | Gravity (gravity.fast) sells "AI Agents for Business"; Google Antigravity; gravity.com 1993, .ai 2017, .dev 2019, .app 2021 |
| **Germ** | Contested in social software | Germ Network, Mozilla pre-seed 2024, E2EE DMs shipped inside Bluesky Feb 2026, germ.network 2020; germ.com 1997; ~541 US marks match GERM*; npm and PyPI taken; GitHub username taken |
| **Geist** | Vercel's typeface family | Geist Sans/Mono/Serif/Pixel, `npm install geist` at 1.9.0, Google Fonts, vercel/geist-font; geist.com 1995, .ai 2018, .dev 2019, .app 2024, .network 2026-05-15 |
| **Garth** | Live AI engineering product | heygarth.ai by Darner AI ("engineering intelligence platform for AI-assisted development"); every short domain taken |
| **Gamut** | Direct category collision | gamut.so, "the agentic operating system"; gamut.com/.ai/.dev/.app all taken, npm/PyPI/GitHub org all taken |
| **Gusto** | Payroll and HR SaaS of that name | every domain, npm, PyPI and the GitHub org taken |
| **Glint** | Employee-engagement product, acquired by Microsoft | every domain but one, npm, PyPI and the GitHub org taken |
| **Grail** | Biotech company of that name | every domain, npm and PyPI taken |
| **Grasp** | No free namespace | every domain, npm and PyPI taken |
| **Grind** | No free namespace, and reads as grind culture | every domain but one, npm and PyPI taken |
| **Gyro** | Crowded, two pronunciations, and says nothing about the mark | GYRO travel-rights platform (gyro.com 1998), Gyro AI Labs, gyro group; npm, PyPI, GitHub org taken; free only on .io/.so/.co/.gg/.sh |
| **Grove, Glade, Gasket** | Domains and packages gone | all `.com/.ai/.dev/.app` taken; Glade is GTK's UI designer and a household brand |

## Alive

| Name | Namespace | Notes |
|---|---|---|
| **Trilobe** | `.com` taken (French watchmaker), `.ai/.dev/.app` free, npm/PyPI/GitHub free | The shape's own name. No software collision found |
| **Trilobate** | Same as above, all free but `.com` | The botanical word for three-lobed. Long, and a mouthful aloud |
| **Germinal** | `.com/.ai` taken, `.dev/.app` free, npm/PyPI/GitHub free | German's earliest stage of development. Small takers: a CEA nuclear fuel code, and Tamarind Bio's antibody-design tool. No community or agent platform |
| **Germline** | `.dev` and `.com` taken, `.ai` free, registries free | The biology meaning is strong but the namespace is half gone |
| **Germin** | `.ai/.dev` free, npm taken | Short, but `-in` endings read pharmaceutical |

## Recommendation

**Trilobe** as the product name, **Germ** as the codename the repo already uses.
The mark is a trilobe, the word is the mark, and nothing in software owns it.
If it has to be a word that means something before you explain it, **Germinal**,
said aloud as "Germ".
