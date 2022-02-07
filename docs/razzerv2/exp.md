# Evaluation plan

## Finding real bugs

## Effectiveness of the context-sensitive coverage (or two pair of comms) compared with a single data race

### How many of known CVEs are context-sensitive?

#### Both comms
- CVE-2019-6974
- CVE-2019-1999
- CVE-2019-2025

#### Only one comm
- e20a2e9c
- 11eb85ec

#### Our arguments

- There are many such race conditions

### During fuzzing a specific subsystem (i.e., KVM) to find above bugs

#### Given an offending comm (i.e., dedup effectiveness)

- # of the total detection of the comm
- Among the total detected comms, # of comms that can trigger the bugs
- # of unique comm with respect to the context
  - w/ or w/o the module proximity
- Among the unique comms, # of unique comms that can trigger the bugs

#### Our arguments

- Dedup is very effective to reduce unnecessary executions

### The number of total runs (or the spending time) until finding above bugs

- Random selection amomg detected instances of comms
- Snowboard
- Razzer V2

#### Our arguments

- TBD
