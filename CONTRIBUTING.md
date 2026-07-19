# Contributing

RLViz is early. Issues and focused pull requests are welcome, especially around real trajectory formats and inspection workflows.

Before starting a larger change, open an issue describing the user workflow and the source format or behavior involved.

## Development

Requirements:

- Go 1.24 or newer
- Node.js 22 or newer

Run the local checks:

```bash
make web-install
make check
make build
```

Protocol changes must include updated schemas, fixtures, and conformance tests. Avoid committing proprietary traces, credentials, model reasoning, customer data, or generated caches.

RLViz is a public repository. Keep brainstorming transcripts, AI handoffs,
rejected design studies, and other internal working material outside the
repository. Public docs should capture accepted product behavior and durable
decisions rather than the private process that produced them.

## Pull requests

Keep changes scoped and explain the concrete workflow they improve. Include verification commands and screenshots for visible UI changes.
