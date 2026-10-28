---
type: Log Entry
title: "The server gzips compressible responses"
description: "The server gzips a compressible response when the client sends Accept-Encoding: gzip: text pages, JSON, JavaScript and SVG, including the aggregate search-index.json, when the body is large enough to be worth it."
tags: [log, update, perf]
kind: Update
status: stable
---

The server gzips a compressible response when the client sends `Accept-Encoding: gzip`: text pages, JSON, JavaScript and SVG, including the aggregate `search-index.json`, when the body is large enough to be worth it. Images, fonts, small bodies and range requests pass through uncompressed. The compressed and uncompressed forms carry separate `ETag`s and `Vary: Accept-Encoding`, and a `304` carries the tag of the coding the response would use, so a cache keeps the two codings apart. `design/rendering.md` says so.
