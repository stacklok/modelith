# Self-contained HTML viewer

A domain model needs an explorable, higher-fidelity view than the deliberately lossy Mermaid diagram (ADR-0002). `render --format html` writes a single offline file with a deterministic grid of entities, relationships, and is-a links, plus details and model-wide vocabulary. The HTML golden lives in `internal/render/html/testdata/`, not alongside the compact worked example, because the embedded viewer would overwhelm its source and Markdown.

The viewer embeds small, handwritten CSS and vanilla JavaScript in the binary and the output file. A server would make a shared file require deployment; a vendored JS graph library would add a build/runtime supply-chain dependency for a layout a small grid can provide. Import definitions remain outside this file; qualified references appear as external nodes.

Models, including vendored copies, are untrusted. Go `encoding/json` serializes the data into an `application/json` script with its default HTML escaping, preventing script-tag breakout. JavaScript reads that data and inserts model text only through DOM text APIs, never HTML parsing. The renderer and viewer perform no network requests. `TestADR_0020_SelfContainedHTML` pins the offline asset and DOM boundary.
