<!-- nap-deck: v2 -->

<!-- id: linux-paging-4level-walk -->
<!-- type: basic -->

Prompt:
Describe a 4-level page table walk on x86_64.

Answer:
1. CR3 points to the PML4 base.
2. The virtual address indexes PML4, PDPT, PD, and PT.
3. The final PTE yields the frame and flags.

Explanation:
The hardware page walker resolves one level at a time until the final PTE yields the physical frame and permission bits.

Tags:
linux/mm, x86_64/paging

+++

<!-- id: mmap-private-anon -->
<!-- type: code-cloze -->

Prompt:
Which flags create an anonymous private mapping?

```c
void *p = mmap(NULL, 4096, PROT_READ | PROT_WRITE, {{?}}, -1, 0);
```

Options:
- MAP_SHARED
- MAP_FIXED
- MAP_PRIVATE | MAP_ANONYMOUS
- MAP_HUGETLB

Answer:
MAP_PRIVATE | MAP_ANONYMOUS

Explanation:
`MAP_PRIVATE | MAP_ANONYMOUS` creates a private, non-file-backed mapping.

Tags:
linux/mm, mmap, virtual-memory

+++

<!-- id: boot-path-order -->
<!-- type: ordered-recall -->

Prompt:
Order the major steps from power-on to the kernel entry point on a typical x86_64 Linux boot path.

Options:
- Firmware initializes hardware and selects a boot target.
- The bootloader loads the kernel image and initrd into memory.
- The kernel decompresses, sets up early paging, and initializes subsystems.
- Control transfers to the kernel entry point and early init continues.

Explanation:
This card is for rehearsing the sequence itself, not just recognizing individual boot components.

Tags:
x86_64/boot, linux/init, firmware
