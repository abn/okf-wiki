---
type: Guide
title: First run
description: A page two directories deep, which is what the test bed's nested section exists to prove.
tags: [testbed, nested]
---

# First run

This page lives at `testbed/nested/setup/first-run.md`, which is two levels
below the bundle root. It is rendered as a page of the `testbed` section, and
its slug carries the path, so it is written to
`/wiki/testbed/nested/setup/first-run.html`.

A page is a page wherever it sits in the bundle. A nested directory does not
become a section of its own, it does not appear separately in the sidebar, and a
section that has no `index.md` gets no entry of its own in the sidebar. What
changes is only the URL and the ordering: the page is sorted with its siblings
by title.

Links work the same from any depth. This one goes back up two levels to a page
in another section:

[The rendering pipeline](../../../design/rendering.md)

and this one goes up one level, to a sibling of this page's parent. The
intermediate directory has no page of its own, so this lands on the section it
belongs to:

[Back to the test bed](../../index.md)

which is the one case a reader should not mistake for a section: a link to
`../index.md` from here is a page, and the sidebar entry above it is the
section, but the two are the same file.

A relative link that climbs out of the bundle entirely is a different case, and
is covered in [Blocks](../../blocks.md#links).
