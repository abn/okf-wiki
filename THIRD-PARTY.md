# Third-party notices

okf-wiki itself is MIT licensed; see [LICENSE](LICENSE). That license does not
extend to what this project bundles.

okf-wiki bundles third-party work in two places: self-hosted fonts served with
every page, and the offline Mermaid diagram runtime baked into the container
image. This file records those components and their licenses.

## Fonts (SIL Open Font License 1.1)

Bundled with the default `paper` theme, in
`internal/wiki/themes/default/assets/fonts/`:

- **Figtree**: Copyright 2019 The Figtree Project Authors
  (https://github.com/googlefonts/figtree). Licensed under the SIL Open Font
  License, Version 1.1 (full text below). Variable weight 300 to 900.
- **Inter**: Copyright 2016 The Inter Project Authors
  (https://github.com/rsms/inter). Licensed under the SIL Open Font License,
  Version 1.1 (full text below). Variable weight 100 to 900.
- **JetBrains Mono**: Copyright 2020 The JetBrains Mono Project Authors
  (https://github.com/JetBrains/JetBrainsMono). Licensed under the SIL Open Font
  License, Version 1.1 (full text below). Variable weight 400 to 800.

A theme may bundle its own faces, in which case that theme's licence applies to
them and they are not covered by this file.

The OFL permits bundling and redistribution. It requires that the fonts are not
sold on their own, that any Reserved Font Name is not used by modified versions,
and that this license text and the copyright notices accompany the fonts. The
woff2 files are vendored unmodified from the Google Fonts css2 API, so no
Reserved Font Name condition is triggered.

The latin subset is vendored and the other five subsets Google returns are not.
The site is English technical writing, and dropping unused subsets is the point
of self-hosting rather than a shortcut against it.

## Diagram runtime (`mermaid/mermaid-bundle.min.mjs`)

The container image builds a single ESM bundle from pinned packages. The bundle,
and the transitive dependencies it inlines, are third-party:

| Component | Version | License |
| :--- | :--- | :--- |
| mermaid | 11.17.2 | MIT |
| @mermaid-js/layout-elk | 1.0.0 | MIT |
| elkjs | 0.9.3 | EPL-2.0 |
| katex | 0.16.47 | MIT |

Transitive dependencies are predominantly MIT and ISC, with a small number of
BSD-3-Clause, Apache-2.0, MPL-2.0, EPL-2.0, and Unlicense components. Full
license texts for every bundled package are available in the published npm
packages named by `mermaid/package-lock.json`.

### MIT License

The MIT License text below applies to mermaid, @mermaid-js/layout-elk, katex,
and the MIT-licensed transitive dependencies.

```
MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## SIL Open Font License, Version 1.1

Copyright © 2017 IBM Corp. with Reserved Font Name "Plex"

This Font Software is licensed under the SIL Open Font License, Version 1.1.

This license is copied below, and is also available with a FAQ at: http://scripts.sil.org/OFL


-----------------------------------------------------------
SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007
-----------------------------------------------------------

PREAMBLE
The goals of the Open Font License (OFL) are to stimulate worldwide
development of collaborative font projects, to support the font creation
efforts of academic and linguistic communities, and to provide a free and
open framework in which fonts may be shared and improved in partnership
with others.

The OFL allows the licensed fonts to be used, studied, modified and
redistributed freely as long as they are not sold by themselves. The
fonts, including any derivative works, can be bundled, embedded,
redistributed and/or sold with any software provided that any reserved
names are not used by derivative works. The fonts and derivatives,
however, cannot be released under any other type of license. The
requirement for fonts to remain under this license does not apply
to any document created using the fonts or their derivatives.

DEFINITIONS
"Font Software" refers to the set of files released by the Copyright
Holder(s) under this license and clearly marked as such. This may
include source files, build scripts and documentation.

"Reserved Font Name" refers to any names specified as such after the
copyright statement(s).

"Original Version" refers to the collection of Font Software components as
distributed by the Copyright Holder(s).

"Modified Version" refers to any derivative made by adding to, deleting,
or substituting -- in part or in whole -- any of the components of the
Original Version, by changing formats or by porting the Font Software to a
new environment.

"Author" refers to any designer, engineer, programmer, technical
writer or other person who contributed to the Font Software.

PERMISSION & CONDITIONS
Permission is hereby granted, free of charge, to any person obtaining
a copy of the Font Software, to use, study, copy, merge, embed, modify,
redistribute, and sell modified and unmodified copies of the Font
Software, subject to the following conditions:

1) Neither the Font Software nor any of its individual components,
in Original or Modified Versions, may be sold by itself.

2) Original or Modified Versions of the Font Software may be bundled,
redistributed and/or sold with any software, provided that each copy
contains the above copyright notice and this license. These can be
included either as stand-alone text files, human-readable headers or
in the appropriate machine-readable metadata fields within text or
binary files as long as those fields can be easily viewed by the user.

3) No Modified Version of the Font Software may use the Reserved Font
Name(s) unless explicit written permission is granted by the corresponding
Copyright Holder. This restriction only applies to the primary font name as
presented to the users.

4) The name(s) of the Copyright Holder(s) or the Author(s) of the Font
Software shall not be used to promote, endorse or advertise any
Modified Version, except to acknowledge the contribution(s) of the
Copyright Holder(s) and the Author(s) or with their explicit written
permission.

5) The Font Software, modified or unmodified, in part or in whole,
must be distributed entirely under this license, and must not be
distributed under any other license. The requirement for fonts to
remain under this license does not apply to any document created
using the Font Software.

TERMINATION
This license becomes null and void if any of the above conditions are
not met.

DISCLAIMER
THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO ANY WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT
OF COPYRIGHT, PATENT, TRADEMARK, OR OTHER RIGHT. IN NO EVENT SHALL THE
COPYRIGHT HOLDER BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
INCLUDING ANY GENERAL, SPECIAL, INDIRECT, INCIDENTAL, OR CONSEQUENTIAL
DAMAGES, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
FROM, OUT OF THE USE OR INABILITY TO USE THE FONT SOFTWARE OR FROM
OTHER DEALINGS IN THE FONT SOFTWARE.

The license above is reproduced with the IBM Plex copyright notice. It is
identical for JetBrains Mono, whose copyright notice appears at the top of this
file.
