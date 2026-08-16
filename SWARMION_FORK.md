# Swarmion-maintained embedded Dolt driver

This branch publishes the patched embedded driver under
`github.com/nustiueudinastea/doltsqldriver/v2` and depends on the maintained
`github.com/nustiueudinastea/dolt/go` module. The distinct paths allow a normal
Go consumer to resolve the complete dependency graph without downstream
`replace` directives.

The branch carries Swarmion's public connector state, deterministic lock
failure, parameter forwarding, and working-set refresh patches. The module
identity and self-import rewrite changes no runtime behavior.

When syncing upstream, retain both maintained module paths, reapply the driver
patches, and run the full driver suite against the corresponding maintained
Dolt commit before publishing a new revision.
