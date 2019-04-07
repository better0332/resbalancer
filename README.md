# resbalancer
K8S Resource Balancer Application

# usage
```bash
K8S Resource Balancer Application.

Usage:
   [flags]

Flags:
      --alsologtostderr                  log to standard error as well as files
  -h, --help                             help for this command
  -c, --kubeconfig string                Kube config path. Only required if out-of-cluster.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --log_file string                  If non-empty, use this log file
      --logtostderr                      log to standard error instead of files (default true)
  -r, --ratio float                      sensitivity ratio [1,n] (default 2)
  -p, --recycle_period duration          Recycle period. (default 2m0s)
      --skip_headers                     If true, avoid header prefixes in the log messages
      --stderrthreshold severity         logs at or above this threshold go to stderr
  -v, --v Level                          number for the log level verbosity
      --version                          version for this command
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging

```