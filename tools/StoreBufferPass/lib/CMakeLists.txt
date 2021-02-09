add_library(SSBPass SHARED SSBPass.cpp)

target_include_directories(SSBPass
  PRIVATE "${PROJECT_SOURCE_DIR}/include"
  )

