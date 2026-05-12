package media

// ChainTransformers chains two transformers together. Either or both may be nil. Returns nil if both are nil.
func ChainTransformers(t1, t2 Transformer) Transformer {
	if t1 == nil {
		return t2
	}
	if t2 == nil {
		return t1
	}
	return &TransformerChain{
		t1, t2,
	}
}

type TransformerChain struct {
	t1, t2 Transformer
}

func (c *TransformerChain) Transform(bts []byte) ([]byte, error) {
	bts, err := c.t1.Transform(bts)
	if err != nil {
		return nil, err
	}
	return c.t2.Transform(bts)
}
