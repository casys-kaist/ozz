set(CMAKE_CROSSCOMPILING TRUE)
set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR aarch64)

set(TARGET "aarch64-linux-gnu")

set(CMAKE_C_COMPILER_TARGET ${TARGET})
set(CMAKE_C_COMPILER clang)
set(CMAKE_CXX_COMPILER_TARGET ${TARGET})
set(CMAKE_CXX_COMPILER clang++)
set(CMAKE_ASM_COMPILER_TARGET ${TARGET})
set(CMAKE_ASM_COMPILER clang)

set(ARCH_FLAGS "-target aarch64-linux-gnueabi")
set(CMAKE_C_FLAGS "-Wall -Wextra ${ARCH_FLAGS}")
set(CMAKE_CXX_FLAGS "-Wall -Wextra -std=c++11 -fno-exceptions -fno-threadsafe-statics ${ARCH_FLAGS}")

add_executable(arm-barrier-simple
  arm-barrier-simple.c)

target_link_libraries(arm-barrier-simple)
