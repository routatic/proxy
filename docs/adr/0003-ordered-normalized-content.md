# Normalized content keeps client block order

The canonical request model uses one ordered list of content blocks for system
and message content. Text, images, thinking, tool calls, tool results, and cache
directives stay at their original positions. This requires a larger core
refactor, but it prevents separate convenience fields from changing request
order or losing cache boundaries. Unknown block types remain as ordered raw
JSON for compatible provider formats and are never silently dropped.
