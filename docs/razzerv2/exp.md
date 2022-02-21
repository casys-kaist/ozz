# Evaluation plan

## Finding real bugs

## How much RazzerV2 is faster than previous scheduling guided fuzzer (i.e., Snowboard, Razzer) when finding known bugs

- We want to compare the execution number until finding those bugs
- And then analyze why the number is derived (i.e., how much new approaches affect the number)

### Target bugs

#### Involving context-sensitive comms

- Both comms
  - CVE-2019-6974
  - CVE-2019-1999
  - CVE-2019-2025
- Only one comm
  - e20a2e9c
  - 11eb85ec

#### Involving context-oblivious comms only

- CVE-2017-2636

### The total execution number until finding known bugs

- During fuzzing a specific subsystem (i.e., KVM) to find above bugs
- We measure the number of tested interleavings
  - But we measure the number of all executions

### Analysis

#### Effectiveness of the context-sensitive coverage compared with a single data race (i.e., dedup effectiveness)

- We want to argue that the context-sensitive coverage is effective to
  select syscall pairs to test
- We measure the number of syscall pairs tested by RazzerV2 compaired
  with Razzer and Snowboard

##### Details 

- TBD

#### Effectiveness of the scheduling mechanism in completely testing interleavings between two calls

- We want to argue that our scheduling mechanism (i.e., testing
  interleaving) is effective to cover all coverage comparied to Razzer
  and Snowboard (i.e., testing a single)

##### Details

- TBD
