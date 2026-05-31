# Flashcards roadmap

Nap now ships with a first-party flashcards system designed for systems and computer engineering study.

## Goals

- own the review flow inside Nap
- keep decks editable as normal files
- track progress per card instead of per deck file
- support systems-heavy study patterns such as ordered recall, code reading, and Linux/kernel internals

## Current state

### Native Nap decks

Nap-owned decks use a dedicated filename:

- `00-nap-cards.md`

Review happens inside Nap, and per-card progress is stored in a hidden sidecar state file next to the deck.

Current behavior:

- `f` scaffolds `00-nap-cards.md`
- `g` appends a draft basic card from the selected snippet and selects the deck for editing
- `F` starts in-app review for the selected folder
- `z` resets native progress
- folder dashboards and tree indicators reflect native review state
- result summaries distinguish `correct`, `recall`, `incorrect`, and `pending`

### Shipped card types

- `basic`
- `single-choice`
- `multi-choice`
- `code-cloze`
- `ordered-recall`
- `trace`

## Native deck format

Native decks are Markdown files with a Nap deck header and repeated card blocks separated by `+++`.

```md
<!-- nap-deck: v2 -->

<!-- id: boot-path-order -->
<!-- type: ordered-recall -->

Prompt:
Order the major steps from power-on to the kernel entry point on a typical x86_64 Linux boot path.

Options:
- Firmware initializes hardware and selects a boot target.
- The bootloader loads the kernel image and initrd into memory.
- The kernel decompresses, sets up early paging, and initializes subsystems.
- Control transfers to the kernel entry point and early init continues.
```

## Planned phases

1. **Foundation** ✅
   Native-only napcards, deck scaffolding, JSON sidecar persistence, reset flow, folder dashboards, tree indicators, and the in-app review loop are shipped.
2. **Rich card core** ✅
   v2 deck parsing plus `basic`, `single-choice`, `multi-choice`, `code-cloze`, `ordered-recall`, and `trace` cards are implemented.
3. **Systems drill cards** ✅
   Trace cards now cover kernel, CPU, syscall, MMU, and boot-flow reasoning where the learner follows a path or state transition without auto-execution.
4. **Authoring workflow**
   Add richer deck examples and expand drafting helpers beyond the first snippet-to-card flow.

Current authoring progress:

- validation feedback in Nap is shipped
- the default deck now includes examples for `basic`, `code-cloze`, `single-choice`, `multi-choice`, `ordered-recall`, and `trace`
- `g` appends a draft basic card from the selected snippet into the folder deck

Next authoring step:

- add more specialized draft helpers beyond the basic snippet-to-card flow
5. **Study dashboard**
   Show due-by-topic, weak areas, recent lapses, and per-folder study health in the folder dashboard.
6. **Analytics**
   Track retention by topic, study session history, and long-term weak-area trends.

## Open design items

- move tracker storage from JSON sidecar state to a richer database if the feature set outgrows the simple format
- expand trace-card variants beyond the initial choice-or-reveal model if systems drills need richer branch/path checks
