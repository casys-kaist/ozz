#ifndef __DECL_TSO_H
#define __DECL_TSO_H

#define MEMORYMODEL tso
#define STORE_CALLBACK_IMPL __store_callback_tso
#define LOAD_CALLBACK_IMPL __load_callback_tso
#define FLUSH_CALLBACK_IMPL __flush_callback_tso
#define FEEDINPUT_CALLBACK_IMPL __feedinput_callback_tso
#include "runtime/_decl_callback.h"

#endif // __DECL_TSO_H
