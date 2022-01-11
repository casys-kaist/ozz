# Topic of this project
  - Scheduling guiding fuzzer

# What is the problem
  - We want to collect coverage of interleavings
    - To determine whether we need to keep mutating the given input
  - Based on the coverage, guiding the fuzzer
    - To let a program experience a given coverage (analogous to branch flipping)

# Goal of this project:
  - Abstract the interleaving
    - Must be
    - Simple
    - Informative
  - Scheduling to expand the coverage

# Problem statements
  - Fuzzer requires a coverage capturing interesting interleavings
    - Interesting interleavings must capture the combination of communication
  - Based on the coverage, guide the fuzzer to quickly expand the coverage
    - Q) Do we argue a contribution on this point? It seems Razzer, Snowboard,
    and SKI share the exactly same mechanism (at least theoretically)?

# Challenges
  - Combination of all communications are numeorous
    - Many communications
  - Interleaving patterns are not standardized

# Observation
  - Observation 1: Kernel is separated into multiple subsystems
    - Each subsystem does its own jobs
  - Each subsystem tends to access their own memory objects
    - Another subsystem calls their APIs when required
  - Observation 2: A concurrency rule is typically bound to each subsystem
    - Called the (spatial) locality property of concurrency rules
    - It is very unlikely that, for example, the VFS layer acquires a lock and then,
    a specific subsystem releases the lock
    - Q) Or do we need to say this as a hypothesis?
  - Q) Conway's law?
  - Observation 3: A concurrency failures are typically caused by few communications
    - 3 communications in most cases
  - Observation 4: Data races are often independent with each other
    - Their manifestation does not affect each other
  - Observation 5: a hierarchical relationship (?)
    - Perhaps we don't want to exploit this

# Our approach to abstract interleaving
  - Focusing a specific subsystem at a time
  - API calls to other subsystems, a caller of functions in the
    subsystem, subsequent executions can be think as aggregated virtual instructions
  - Interleaving patterns
    - At most 3 races (and at most 6 (= 3 * 2) memory accesses)
  - 

# Our approach to guide the fuzzer
  - Coalesce independent races
    - Dependencies
      - Race-steered control flow

# Concerns
  - Are there cases that a specific interleaving in a subsystem causes a
    race-steered control flow which in turn invokes another subsystem,
    and then the subsytem races resulting in a failure.
    - Unless we find such cases, we are not gonna take consider about it
