---
type: Guide
title: Diagrams
description: Mermaid diagrams the renderer passes through to the browser, in the shapes a docs bundle actually uses.
tags: [testbed, diagrams]
---

# Diagrams

A `mermaid` fence is not highlighted. It passes through as a `pre.mermaid` block
that the diagram runtime picks up in the browser, so a diagram only appears once
that runtime has loaded. Everything below is drawn client side, which is why the
page is readable with the runtime absent and the diagrams are not.

## Flowchart

```mermaid
flowchart TD
    A["docs/*.md"] --> B{"frontmatter<br/>present?"}
    B -->|yes| C["title, description,<br/>tags, status"]
    B -->|no| D["first H1 is the title"]
    C --> E["goldmark parse"]
    D --> E
    E --> F["walk the AST:<br/>links, callouts, heading ids"]
    F --> G["render to HTML"]
    G --> H["write the page"]
    G --> I["append a search chunk"]
```

## Sequence

```mermaid
sequenceDiagram
    participant U as Browser
    participant S as okf-wiki serve
    participant F as out/wiki
    U->>S: GET /wiki/
    S->>F: read index.html
    F-->>S: 200 text/html
    S-->>U: page + ?v= asset links
    U->>S: GET /wiki/search-index.json
    Note over U: loaded once, on first focus
    U->>U: filter chunks locally
```

## State

```mermaid
stateDiagram-v2
    [*] --> Unresolved
    Unresolved --> Resolved: ResolveTheme
    Resolved --> Compiled: parse template, fingerprint
    Compiled --> Emitted: WriteTo
    Emitted --> [*]
    Resolved --> Failed: unknown asset mode
    Compiled --> Failed: template does not parse
    Failed --> [*]
```

## Wide, to check the lightbox

A diagram wider than the reading column is the case the full-screen viewer
exists for. Click any diagram on this page to open it.

```mermaid
flowchart LR
    n0["node 0"] --- n1["node 1"] --- n2["node 2"] --- n3["node 3"]
    n3 --- n4["node 4"] --- n5["node 5"] --- n6["node 6"] --- n7["node 7"]
    n7 --- n8["node 8"] --- n9["node 9"] --- n10["node 10"] --- n11["node 11"]
    n0 -.-> n6
    n3 -.-> n9
```

## Syntax that must not break the page

An invalid diagram leaves its source visible rather than blanking the block,
so the mistake is readable in place.

```mermaid
this is not valid mermaid
```
