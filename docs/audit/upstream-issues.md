# Upstream framework issues

An entry identifies a reproducible problem in a framework contract or implementation, the versions exercised, and the smallest framework change that would address it. Flame owns its product policies, including Session and Run lifecycle, persisted history, receipts, accounting, and presentation. An integration defect in those policies belongs in Flame.

## Ownership

| Project | Framework responsibility |
| --- | --- |
| Scope | AI contracts, provider protocols, and durable agent execution. [Scope boundaries](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/README.md#boundaries) |
| Dougong | TypeScript plugin composition, atomic host publication, resource lifetimes, and observation. [Dougong packages](https://github.com/Tangerg/dougong/blob/85ca855ada3709484bd8984d21cbe0b17108a91a/README.md) |
| Oolong | Terminal rendering, input, scene identity, streaming content, and output admission. [Oolong architecture](https://github.com/Tangerg/oolong/blob/4d49204c89a66adac6c0767457b41e139e6c7ebf/README.md#build-from-layers) |
| Frame | Plugin contracts, frozen service bindings, invocation evidence, and managed resource lifetimes. [Frame responsibilities](https://github.com/Tangerg/frame/blob/a649f2a498f0e706ce57ab092b60d71292491eb0/README.md) |

Framework changes use framework-native types and standalone fixtures. Flame translates those contracts into its product model and verifies the integration against released modules.

No framework issue is open.
