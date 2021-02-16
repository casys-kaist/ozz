#ifndef __DECL_PSO_H
#define __DECL_PSO_H

#define MEMORYMODEL pso
#define STORE_CALLBACK_IMPL __store_callback_pso
#define LOAD_CALLBACK_IMPL __load_callback_pso
#include "runtime/_decl_callback.h"

#endif // __DECL_PSO_H
