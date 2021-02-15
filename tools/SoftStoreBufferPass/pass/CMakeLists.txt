set(SSBPass_src
  SSBPass.cpp
  )

add_library(SSBPass SHARED ${SSBPass_src})

# Run clang-format before build
add_custom_command(TARGET SSBPass
  PRE_BUILD
  DEPENDS ${SSBPass_src}
  COMMAND clang-format -i -style=file ${SSBPass_src}
  WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
  )

target_include_directories(SSBPass
  PRIVATE "${PROJECT_SOURCE_DIR}/include"
  )

install(TARGETS SSBPass DESTINATION lib)
