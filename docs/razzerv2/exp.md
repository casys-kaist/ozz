# Evaluation plan

## How to show two pair (or considering the context) is better than a single communication

## Context-sensitive bugs

### How many of know CVEs are context-sensitive?

### Both comms
- CVE-2019-6974
- CVE-2019-1999
- CVE-2019-2025

### Only one comm
- e20a2e9c
- 11eb85ec

## During fuzzing a specific subsystem (i.e., KVM)

### Given an offending comm (i.e., dedup effectiveness)

- # of the total detection of the comm
- # of unique comm with respect to the context
  - w/ or w/o the module proximity

### The number of total runs until finding above bugs

- Random guiding
- Snowboard
- Razzer V2
