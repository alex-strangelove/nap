<!-- nap-flashcards:v1 -->

---
id: linux-paging-4level-walk
tags:
  - linux/mm
  - x86_64/paging
---
Q:
Describe a 4-level page table walk on x86_64.

A:
1. CR3 points to the PML4 base.
2. The virtual address indexes PML4, PDPT, PD, and PT.
3. The final PTE yields the frame and flags.

---
id: syscall-vs-trap
tags:
  - linux/syscalls
  - cpu/control-flow
---
Q:
What is the difference between a userspace syscall transition and a synchronous CPU trap?

A:
A syscall is an intentional privilege transition requested by software. A synchronous trap is raised by the CPU while executing an instruction, for example on divide-by-zero or a page fault.
