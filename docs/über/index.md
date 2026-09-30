---
type: Guide
title: Über
description: A section whose directory name starts outside ASCII, which is how the test bed proves section titles survive a non-ASCII name.
tags: [testbed, unicode]
---

# Über

This section's directory is `docs/über`. The name begins with a multi-byte
character, which is the case that broke section titles: the display title used
to be built by upper-casing the first *byte* of each word, and a byte slice of a
multi-byte character is not a character, so this section was titled with a
replacement character followed by "ber".

The directory name is also part of every URL in the section, so a page here is
served from a path containing the same character. That works because the
renderer writes the file under the name the bundle gave it and the browser
percent-encodes the request.

## Warum diese Seite existiert

Ein Sektionsname darf ein beliebiges Unicode-Zeichen enthalten. Der Titel wird
pro Silbe grossgeschrieben, und der erste Buchstabe wird als Zeichen behandelt,
nicht als Byte. Deutsch, 中文, русский and Ελληνικά are all fine as a first
letter.

Ünicode is the word that breaks it if the first letter is not ASCII: Ü is two
bytes, and upper-casing one of them is not upper-casing the letter.

## Was das für Links heisst

A link to this section resolves like any other, and the path keeps the
character:

[Zurück zur Übersicht](../index.md)

The slug is percent-encoded on the wire and the file on disk keeps the raw
character, which is what lets the two agree.
