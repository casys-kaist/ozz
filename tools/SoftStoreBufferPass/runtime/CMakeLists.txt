set(PSO_src
  PSOCallbacks.cpp
  )

set(TSO_src
  TSOCallbacks.cpp
  )

add_library(TSOCallbacks ${TSO_src})
add_library(PSOCallbacks ${PSO_src})

# Run clang-format before build
add_custom_command(TARGET PSOCallbacks
  PRE_BUILD
  DEPENDS ${PSO_src}
  COMMAND clang-format -i -style=file ${PSO_src}
  WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
  )

add_custom_command(TARGET TSOCallbacks
  PRE_BUILD
  DEPENDS ${TSO_src}
  COMMAND clang-format -i -style=file ${TSO_src}
  WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
  )

include_directories("${PROJECT_SOURCE_DIR}/include")

install(TARGETS TSOCallbacks PSOCallbacks
  DESTINATION lib
  )
