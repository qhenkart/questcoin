package blockchain

// TxOutput indivisible outputs, You cannot reference part of an output. eg. you can't take a $10 bill and split it in half to give change. You would have to make 2 new outputs with 5 each
type TxOutput struct {
	// value in tokens, assigned and locked in the output
	Value int
	// a value necessary to unlock the tokens locked in the value field
	//
	// in BTC this is implemented in Script lang
	PubKey string
}

// TxInput refences to pevious outputs
type TxInput struct {
	// references the transaction that the output is inside of
	ID []byte
	// Index where the output appears. If the transaction has 3 outputs but we want to reference only 1. then we know transaction ID: x at index 2
	Out int
	// similiar to pubkey. Provides the data that is used in the outputs pubkey
	Sig string
}

// CanUnlock checks to see if the data matches the signature. If they come back as true,
// then the account owns the data inside the output referenced by the input
func (in *TxInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// CanBeUnlocked checks to see if the data matches the pubkey. If they come back as true,
// then the account owns the data inside the output
func (out *TxOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
