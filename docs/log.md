# Change log

The change log records changes to this documentation bundle: pages added, renamed,
restructured, or deprecated. It is not the software changelog; a release note
belongs with the release, and this log records the pages and the behaviour they
describe.

Each change is one entry page at `logs/YYYY/MM/DD-<slug>.md`, and the entry pages
are the log. The rail groups them by year and month, and search finds one by a page,
a file name, or a phrase from the change; no index lists them. This page is written
by hand, and no tool regenerates it.

A month directory carries an `index.md` whose first heading names the month, for
example `# September`. It has no frontmatter and gives the rail its group label; the
entry pages below it carry the history.

An entry page carries:

- `type`: `Log Entry`
- `title`: a short name for the change, under 80 characters
- `description`: one sentence naming the change, used by search
- `kind`: `Creation`, `Update`, `Deprecation`, or `Initialization`
- `tags`: `log`, the kind in lower case, and a finer label such as `feature` or
  `fix` where the change has one
- `status`: `stable`

The body carries the detail and links to the pages the change touched. A relative link
from an entry page climbs three levels, for example `../../../usage/cli.md`.

To add an entry, copy the most recent page and change its date, slug, fields, and
body. There is no entry index to update and no generator to run.
