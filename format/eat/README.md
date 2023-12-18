# EAT - Eluvio Authorization Tokens

## Token encoding formats

### Base Structure

```
+: append bytes/string operator

TOKEN: PREFIX + BODY

PREFIX: 6b
* 3b Type: 1st byte "a" stands for "auth token" 
  {"aun", "unknown"},
  {"aan", "anonymous"},
  {"atx", "tx"},
  {"asc", "state-channel"},
  {"acl", "client"},
* 1b SigType
 {"_", "unknown"},
 {"u", "unsigned"},
 {"s", "ES256K"},
* 2b Format
 {"nk", "unknown"},        
 {"__", "legacy"},         
 {"j_", "json"},           
 {"jc", "json-compressed"},
 {"c_", "cbor"},           
 {"cc", "cbor-compressed"},
 {"b_", "custom"},         

BODY: base58(SIGNATURE + PAYLOAD)

SIGNATURE: pure signature bytes - type is encoded in SigType of prefix above, and length is implied by type
* "ES256K"  : 65b
* "unsigned":  0b

PAYLOAD: TOKENDATA

TOKENDATA: proper token data encoded according to Format
* json:            token data marshalled as json           
* json-compressed: deflate(json) 
* cbor:            token data masrshalled as cbor
* cbor-compressed: deflate(cbor)
```

### Client Tokens with Embedded Server Token

Client tokens ("ac") contain an embedded state channel token created by elvmaster. That embedded token is encoded
within the client token PAYLOAD as follows:

```
PAYLOAD: EMBEDDED_TOKEN + CLIENT_TOKENDATA
EMBEDDED_TOKEN: LENGTH + PREFIX + BODY
LENGTH: varint(len(PREFIX + BODY))
BODY: SIGNATURE + PAYLOAD
```

The signature of a client token is calculated on the above PAYLOAD and therefore includes the server token and its
 signature.

### Legacy-Signed Token

Scenario: evlmaster generates state channel token in new format, client signs it "the old way"

```
ascsccHwDuvRPCBr6NMxQHTF57Qh9VrtQuak2jt6qEFaX36A7rkmmWNujbS8PUuaDzxUqo3JeY6R95xTzbC62WbxccUnDwAjj5rKWuUqaK5xHHhcbMfWEVGUEMFh7qGhnsbzaJwJsxgS6mVAUeHQjgh9EAAzv28d4yyY99CQ2Ug9XNAk27owqLi1TRRokSHFQ5dUZNdk6ZmLkBHEJLjPTyizKyZc4fFYbrc36DtZQRpGyrFSaaZ8JfCNJX6kcSZzxZETg1DnchWQorjLMXThHT7WuS5m3smGDJ7cMc4WyfTRoyosL.RVMyNTZLX0YzVnhlc3JiN256UHhSbndUNkZIcEtDZFN1UVpjZGtxSDd3VXh5cWdjcmthWjF0TEJHR2R6Z2dvQU14YzVMQlVBRVhhZFV6NEt4SzVTbkxXWjdpRTNiWDVK
```

### OTP-Backwards-Compatible State-Channel-Token

Scenario: old clients expect the token to be a JSON struct with a "qid" key.
Solution: wrap new token format inside small JSON struct as follows:

```json
{
  "qid": "iq__3RiwiP7UJJiHxFLbkL46BoVfKWrB",
  "tok": "ascsccHwDuvRPCBr6NMxQHTF57Qh9VrtQuak2jt6qEFaX36A7rkmmWNujbS8PUuaDzxUqo3JeY6R95xTzbC62WbxccUnDwAjj5rKWuUqaK5xHHhcbMfWEVGUEMFh7qGhnsbzaJwJsxgS6mVAUeHQjgh9EAAzv28d4yyY99CQ2Ug9XNAk27owqLi1TRRokSHFQ5dUZNdk6ZmLkBHEJLjPTyizKyZc4fFYbrc36DtZQRpGyrFSaaZ8JfCNJX6kcSZzxZETg1DnchWQorjLMXThHT7WuS5m3smGDJ7cMc4WyfTRoyosL"
}
```

Legacy encoded form:

```
eyJxaWQiOiJpcV9fM1Jpd2lQN1VKSmlIeEZMYmtMNDZCb1ZmS1dyQiIsInRvayI6ImFzY3NjY0h3RHV2UlBDQnI2Tk14UUhURjU3UWg5VnJ0UXVhazJqdDZxRUZhWDM2QTdya21tV051amJTOFBVdWFEenhVcW8zSmVZNlI5NXhUemJDNjJXYnhjY1VuRHdBamo1cktXdVVxYUs1eEhIaGNiTWZXRVZHVUVNRmg3cUdobnNiemFKd0pzeGdTNm1WQVVlSFFqZ2g5RUFBenYyOGQ0eXlZOTlDUTJVZzlYTkFrMjdvd3FMaTFUUlJva1NIRlE1ZFVaTmRrNlptTGtCSEVKTGpQVHlpekt5WmM0ZkZZYnJjMzZEdFpRUnBHeXJGU2FhWjhKZkNOSlg2a2NTWnp4WkVUZzFEbmNoV1FvcmpMTVhUaEhUN1d1UzVtM3NtR0RKN2NNYzRXeWZUUm95b3NMIn0=
```

### Client or Editor Signed tokens requiring client confirmation

The objective is to implement a simple 'app lock' to avoid users copying playout URLs
 
* Client creates ephemeral private/public key (Ethereum SECP)
* Client creates a 'client-signed token' including the address corresponding to the public key of the ephemeral key
* Client signs the token as usual 

The client-signed token has a `cnf` element with `aek` field allowing to specify the address corresponding to the public
key of the ephemeral key.

```
{
      "txh": "0x0000000000000000000000000000000000000000000000000000000000000000",
      "adr": "0x65419c9f653703ed7fb6cc636cf9fda6cc024e2e",
      "spc": "ispc2XW6n11mJXepAW3WmBSZyuPRtEGv",
      "sub": "iusr2QpVishg9QSGU4TW3Nn4g6gYw6TP",
      "iat": "2023-12-12T13:17:32.779Z",
      "exp": "2023-12-12T17:17:32.779Z",
      "cnf": {
        "aek": "0xC96aAa54E2d44c299564da76e1cD3184A2386B8D"
      }
    }
```

For each resource access the client needs to provide the following:

* authorization header/query parameter using the client signed access token (which includes `cnf/aek`)
* separate header or query parameter specifying the confirmation (aka 'proof' of possession) - which is a confirmation token, like below
  * Header: `Authorization: confirmation accsjcoBtHrLNoym...DRsk`
  * Query parameter: `authorization=accsjcoBtHrLNoym...DRsk` (`authorization=confirmation accsjcoBtHrLNoym...DRsk` also works)
* the confirmation token is signed using the ephemeral key created at the first step
* fields `iat` (issuedAt) and `exp` (expires) are mandatory
* prefix for the token type is `acc`

````
    {
      "adr": "0x57549293ae2aed940aa5e2414a09ab74b4ad7381",
      "iat": "2023-12-12T19:03:53.380Z",
      "exp": "2023-12-12T19:08:53.380Z"
    }
    
token string
    accsjcoBtHrLNoymYRittdMQ96z16yQpDgZxfQQQFR2JG2PfFHKHLA7GfYDmwTJe2Uo7bWoaCGFjJ6fPiuy3mtWpFwTda9dhxAHUj7F9GD3YJE9kibnGZnr9YzyhmNu5EQPkE1QmTAMToqDRsk
````

**Note**: multiple `Bearer` tokens are not permitted when using client/editor signed token with confirmation.


### Brainstorming notes

```
BODY: base64(PAYLOAD).base64(SIGNATURE)

PAYLOAD: EMBEDDED_TOKEN + CLIENT_TOKENDATA
EMBEDDED_TOKEN: LENGTH + PREFIX + BODY
LENGTH: 4 bytes (len(PREFIX + BODY))
BODY: SIGNATURE + PAYLOAD

Client Token

TOKEN: aclsb6 + BODY
BODY: OPAQUE_EMBEDDED_TOKEN_STRING.base64(PAYLOAD).base64(SIGNATURE)

TOKEN: aclsb6 + OPAQUE_EMBEDDED_TOKEN_STRING.base64(PAYLOAD).base64(SIGNATURE)
TOKEN: aclub6 + OPAQUE_EMBEDDED_TOKEN_STRING.base64(PAYLOAD) ==> not allowed: if payload, you need to sign!
TOKEN: aclub6 + OPAQUE_EMBEDDED_TOKEN_STRING                 ==> not allowed: is same as just sending the OPAQUE_EMBEDDED_TOKEN_STRING

SIGNATURE: sig(OPAQUE_EMBEDDED_TOKEN_STRING.base64(PAYLOAD))
```
