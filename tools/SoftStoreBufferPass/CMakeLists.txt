cmake_minimum_required(VERSION 3.15.0)

project(SoftStoreBufferPass)

# Set this to a valid LLVM installation dir
set(LLVM_INSTALL_DIR $ENV{LLVM_INSTALL} CACHE PATH "LLVM installation directory")

# Sanity check
set(LLVM_INCLUDE_DIR "${LLVM_INSTALL_DIR}/include/llvm")
if(NOT EXISTS "${LLVM_INCLUDE_DIR}")
  message(FATAL_ERROR
	" LLVM_INSTALL_DIR (${LLVM_INCLUDE_DIR}) is invalid.")
endif()

set(LLVM_CMAKE_FILE "${LLVM_INSTALL_DIR}/lib/cmake/llvm/LLVMConfig.cmake")
if(NOT EXISTS "${LLVM_CMAKE_FILE}")
  message(FATAL_ERROR
    " LLVM_CMAKE_FILE (${LLVM_CMAKE_FILE}) is invalid.")
endif()

# Add the location of LLVMConfig.cmake to CMake search paths (so that
# find_package can locate it)
list(APPEND CMAKE_PREFIX_PATH "${LLVM_INSTALL_DIR}/lib/cmake/llvm/")

find_package(LLVM REQUIRED CONFIG)

message(STATUS "Found LLVM ${LLVM_PACKAGE_VERSION}")
message(STATUS "Using LLVMConfig.cmake in: ${LLVM_INSTALL_DIR}")

message("LLVM STATUS:
  Definitions ${LLVM_DEFINITIONS}
  Includes    ${LLVM_INCLUDE_DIRS}
  Libraries   ${LLVM_LIBRARY_DIRS}
  Targets     ${LLVM_TARGETS_TO_BUILD}"
)

# Set the LLVM header and library paths
include_directories(SYSTEM ${LLVM_INCLUDE_DIRS})
link_directories(${LLVM_LIBRARY_DIRS})
add_definitions(${LLVM_DEFINITIONS})

set(CMAKE_CXX_STANDARD 14 CACHE STRING "")

# Build type
if (NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Debug CACHE
      STRING "Build type (default Debug):" FORCE)
endif()

# Compiler flags
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wall\
    -fdiagnostics-color=always")

# LLVM is normally built without RTTI. Be consistent with that.
if(NOT LLVM_ENABLE_RTTI)
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fno-rtti")
endif()

add_subdirectory(lib)
