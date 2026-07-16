# Python adapter template

Copy this directory, change the manifest name, then implement `probe` and
`stream`. The adapter requires only Python's standard library. Stdout is
reserved for protocol output; diagnostics belong on stderr.
