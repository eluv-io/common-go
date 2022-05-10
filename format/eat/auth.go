package eat

// Grant is the type of grant kinds for authorization access
type Grant string

// Grants defines the kinds of access authorizations.
var Grants = struct {
	Create    Grant
	Access    Grant
	Read      Grant
	Update    Grant
	ReadCrypt Grant
}{
	Create:    "create",
	Access:    "access",
	Read:      "read",
	Update:    "update",
	ReadCrypt: "read-crypt", // read from content crypt
}
