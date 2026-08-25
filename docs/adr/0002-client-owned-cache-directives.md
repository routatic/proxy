# Provider Prompt Cache directives are owned by the client

The proxy preserves cache directives supplied at the request, tool, system, and
message-block levels, but it does not add, move, or infer cache boundaries.
Provider adapters may omit a directive only when their wire format cannot use
it. This favors predictable request behavior over automatic cache tuning.
