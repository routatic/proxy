# User responses take priority over analytics completeness

Completion records are best-effort analytics. The proxy never delays a model
response when the storage queue is full, so it drops the newest analytics
record and reports the drop. During normal shutdown it drains accepted records
until the existing shutdown deadline, then reports any remaining loss. This
trades complete analytics for predictable user-facing latency.
