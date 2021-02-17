cmake_minimum_required(VERSION 3.19)

project(benchmarks)

find_package(Threads REQUIRED)

set(CMAKE_C_COMPILER clang)

add_subdirectory(pso-simple)
add_subdirectory(tso-simple)
