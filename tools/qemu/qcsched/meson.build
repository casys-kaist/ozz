qcsched_ss = ss.source_set()
qcsched_ss.add(when: 'CONFIG_QCSCHED', if_true: files(
  'hcall.c',
  'breakpoint.c',
  'qcsched.c',
  'vmi.c',
  'timer.c',
  'kssb.c',
  'window.c',
  'state.c',
  'exec_control.c',
  'cookie.c',
))

qcsched_ss.add(when: 'CONFIG_QCSCHED_TRAMPOLINE', if_true: files(
  'trampoline.c',
), if_false: files(
  'blocker.c',
))

specific_ss.add_all(when: 'CONFIG_SOFTMMU', if_true: [qcsched_ss])
