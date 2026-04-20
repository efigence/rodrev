package ipset

type Command string

const IPSET_ADD Command = "add"
const IPSET_DEL Command = "del"

type IPsetCmd struct {
	Cmd   Command
	Addr  string
	IPSet string
}
