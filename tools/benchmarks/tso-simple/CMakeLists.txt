add_executable(tso-simple
  tso-simple.c)

target_link_libraries(tso-simple
  ${CMAKE_THREAD_LIBS_INIT})
