# Topic of this project
  - Scheduling guided fuzzer

# Goal of this project
  - We want to collect coverage of interleavings
    - To determine whether we need to keep mutating the given input
  - Based on the coverage, guiding the fuzzer
    - To let a program experience a given coverage (analogous to branch flipping)
  - Ultimately, we want to find more bugs in shorter time

# Problem statements and motivation
  - A fuzzer requires a coverage capturing interesting interleavings
    - Interesting interleavings must capture the combination of communication
    - A single data race does not explain the context of the data race
  - Based on the coverage, guide the fuzzer to quickly expand the coverage
    - Previous works waste the computing power due to focusing on a single data race

# Existing approaches
  - Single data race as a coverage without a context
    - Razzer, Snowboard, KRace
    - does not explain the context on which instructions take place
	- Testing a single coverage at a time
	  - Snowboard, Razzer
	- Or without guiding
	  - KRace

# Our approach to abstract interleaving
  - Focusing a specific subsystem at a time
    - API calls to other subsystems, a caller of functions in the
      subsystem, subsequent executions can be think as aggregated virtual instructions
  - Decomposing a combination of communcations into two communications
    - Analogous to the 2-gram of control flow

# Our approach to guide the fuzzer
  - Testing multiple combinations at a time
    - Many of communications are independent with each other
    - Coalesce independent races
  - Cooperative way of composing possible commbinations
    - Different interleavings can give a clue of concealed
      interleaving

# Component of this project
  - Abstract the interleaving, which must be
    - Simple
    - Informative
  - Scheduling to expand the coverage, which must be
    - Effective

# Challenges
  - Numerous combination of all communications
  - Interleaving patterns are not standardized
  - Guiding a fuzzer with not-well-refined coverage would harm the
    performance of the fuzzer
  - Hidden data races and hidden interleaving

# Terms
  - Communication: read-from + from-read

# Observation
  - Observation 1: Kernel is separated into multiple subsystems
    - Each subsystem does its own jobs
    - Each subsystem tends to access their own memory objects
    - Another subsystem calls their APIs when required
    - Analogous to the encapsulation of OOP
  - Observation 2: A concurrency rule is typically bound to each subsystem
    - Called the module proximity of concurrency rules
    - It is very unlikely that, for example, the VFS layer acquires a lock and then,
      a specific subsystem releases the lock
    - Q) Hypothesis? Conway's law?
  - Observation 3: A concurrency failures are typically caused by few communications
    - 3 communications in most cases
  - Observation 4: Data races are often independent with each other
    - Their manifestation does not affect each other
  - Observation 5: Two kinds of dependencies
    - Causal dependency: one has a causal effect to another
    - Order dependency: nesting cases
  - Observation 6: A cooperative inference reveals hidden interleaving

# Misc
  - Equivalence relation with respect to a certain data race

# Concerns
  - Are there cases that a specific interleaving in a subsystem causes a
    race-steered control flow which in turn invokes another subsystem,
    and then the subsytem races resulting in a failure
    - Unless we find such cases, we are not gonna take consider about it
    - Perhaps we may fix interleaving in a subsystem and then explore
      interleaving in another subsystem
  - Are there cases that decomposing an interleaving into two
    communications is not suitable to find a failure?
    - Same to the above such that we are not gonna consider this until
      we need it

# Data race in reusable code

## Both
- CVE-2019-6974
- CVE-2019-1999
- CVE-2019-2025

## Only one
- e20a2e9c
- 11eb85ec
