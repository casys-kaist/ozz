;; Ref: https://emacs.stackexchange.com/questions/26093/how-can-i-set-directory-local-variable-in-relative-to-dir-locals-el-file-locati

;; TODO: Make this as a function so that I can reuse across projects
((nil . ((eval . (set (make-variable-buffer-local 'my-project-path)
                      (file-name-directory
                       (let ((d (dir-locals-find-file ".")))
                         (if (stringp d) d (car d))))))
		 ;; I want to use ninja
		 (eval . (set (make-variable-buffer-local 'build-cmd) "ninja"))
		 ;; I want to execute the command in the "./build" directory
		 (compile-command . (format "(cd %s/build; %s)"
									my-project-path
									build-cmd))
         )))
