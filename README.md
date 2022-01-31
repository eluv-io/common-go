# `common-go`: General-Purpose Structs and Helpers

## Package `format`

Package `format` contains general-purpose data types used in the content fabric including IDs, hashes, tokens, etc. Each data type also implements encoders and decoders for the purpose of file storage or exchange in network protocols. In order to ensure that the system can evolve over time without breaking changes, every format is self-describing and follows the definitions of https://github.com/multiformats/multiformats (or at least its principles).

## Package `util`

Package util contains helper functions or general-purpose data structures like LRU caches, queues, worker pools, etc.
