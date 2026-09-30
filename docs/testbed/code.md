---
type: Guide
title: Code
description: Syntax highlighting across languages, and what happens at the edges.
tags: [testbed, code]
---

# Code

Fenced blocks are highlighted at build time. The markup is already coloured in
the file you are served, so highlighting does not depend on JavaScript and does
not shift the page once it runs.

## A language with a lexer

```go
func main() {
	r, err := wiki.New(cfg)
	if err != nil {
		return err
	}
	return r.RenderAll(out)
}
```

```python
def slugify(value: str) -> str:
    keep = [c.lower() for c in value if c.isalnum() or c in " -_"]
    return "".join(keep).replace(" ", "-")
```

```bash
okf-wiki render --content docs --out .scratch/wiki --base /wiki/
```

```yaml
inputs:
  content: docs
  base: auto
  deploy: "true"
```

## A language with no lexer

The block still renders, escaped, with the language kept as a class for a theme
to style. Nothing is lost and nothing fails.

```brainfuck
++++++++[>++++[>++>+++>+++>+<<<<-]>+>+>->>+[<]<-]>>.>---.+++++++..+++.>>.<-.<.+++.------.--------.>>+.>++.
```

## No language

A bare fence is escaped and left unhighlighted.

```
okf-wiki render --content docs
```

## A language name the lexer only guesses at

Falling back to a close match is normal, and the block is still highlighted.

```console
$ make check
gofmt -w cmd internal
go vet ./...
go test ./...
ok  github.com/abn/okf-wiki/internal/wiki
```

## Inline code

Inline spans are escaped and monospaced, with no highlighting:
`renderAll(cfg.SiteDir)`, `--base /wiki/`, `theme.json`.
