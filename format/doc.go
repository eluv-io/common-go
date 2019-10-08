/*
Package format provides encoders and decoders for the various types of data
that need to be serialized/deserialized for the purpose of file storage or
exchange in network protocols.

It is the central place where the default serialization formats for the
different entities is specified. In order to ensure that the system can
evolve over time without breaking changes, every format is self-describing and
follows the definitions of https://github.com/multiformats/multiformats (or at
least its principles).

A note on the GOB and CBOR codecs provided in this package: their format is

	<varint-len><multicodec-path>\n<encoded-data-1>...<encoded-data-n>
	e.g. 5/gob\n<encoded-data-1><encoded-data-2><encoded-data-3>

In contrast, the format produced by the implementations of
github.com/multiformats/go-multicodec (cbor, json, etc.) is:

	<varint-len><multicodec-path>\n<encoded-data-1>...<varint-len><multicodec-path>\n<encoded-data-n>
	e.g. 5/gob\n<encoded-data1>5/gob\n<encoded-data2>5/gob\n<encoded-data3>

The multiformat header is repeated for every encoded item. This obviously
represents a large overhead in the case of a large number of encoded data items
(e.g. the export of a large kv store).
*/
package format
