# LLM Proxy

This context describes the language used to discuss interactive model requests
and their performance through routatic-proxy.

## Language

**Time to First Token (TTFT)**:
For a streaming request, the elapsed time from when the proxy starts reading
the request until it writes the first non-empty text, thinking, or tool content
to the client.
_Avoid_: First-byte time, total response time

**Repeated Conversation**:
A model request that includes a stable prefix from earlier turns plus a new
turn.
_Avoid_: Warm request, duplicate request

**Cache Directive**:
A client-supplied instruction that marks a prompt boundary as eligible for
reuse by the Provider Prompt Cache.
_Avoid_: Automatic cache rule, proxy cache marker

**Cache Usage**:
Provider-reported token counts describing prompt data read from or written to
the Provider Prompt Cache.
_Avoid_: Token Count Cache hit, inferred cache use

**Provider Prompt Cache**:
An upstream provider feature that reuses an unchanged prompt prefix across
model requests.
_Avoid_: Token Count Cache, response cache

**Token Count Cache**:
A process-local cache that reuses tokenizer results without storing raw prompt
text.
_Avoid_: Provider Prompt Cache, response cache

**Completion Record**:
Best-effort analytics data describing one finished model request. Losing this
record must not change the model response seen by the client.
_Avoid_: Response, durable event

**Catalog Snapshot**:
The last successfully loaded set of providers, models, and routing scenarios
used for model selection.
_Avoid_: Live catalog, catalog request

**Known Model**:
A model present in the active configuration or catalog snapshot.
_Avoid_: Requested model, arbitrary model name

**Model Identity**:
The canonical `provider/model` name used to distinguish a model across
providers.
_Avoid_: Short model name, display name

**Unknown Content Block**:
A client content block whose type is not yet modeled by the proxy but whose raw
data and position remain intact.
_Avoid_: Invalid block, ignored block
