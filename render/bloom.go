package render

// BloomOptions tunes [PostChain.AddBloom]. A zero field takes its default,
// so the zero value is a usable bloom.
type BloomOptions struct {
	// Threshold is the brightness (the largest of red, green and blue, 0..1)
	// above which a pixel glows; only the part above it does. Default 0.7.
	Threshold float32
	// Strength scales the glow that is added to the frame. Default 1.
	Strength float32
	// Radius is the distance between the taps of the blur, in pixels of the
	// quarter-size image: 1 blurs over about 8 frame pixels each way. Default
	// 1.
	Radius float32
	// Src is the image whose bright parts glow. The zero value,
	// [TargetChain], is the chain so far: the whole frame. [TargetGroup]
	// makes only the layers of one layer group glow (neon signs, magic,
	// lasers) while equally bright sprites elsewhere do not; the glow is
	// still added to the chain. Any target a pass may read works.
	Src TargetID
}

func (o BloomOptions) withDefaults() BloomOptions {
	if o.Threshold == 0 {
		o.Threshold = 0.7
	}
	if o.Strength == 0 {
		o.Strength = 1
	}
	if o.Radius == 0 {
		o.Radius = 1
	}
	return o
}

// bloomPasses is how many passes a [Bloom] adds to a chain.
const bloomPasses = 5

// Bloom is the handle [PostChain.AddBloom] returns, to retune or switch off
// the effect it added.
type Bloom struct {
	first int // index of its first pass in PostChain.Passes
}

// AddBloom appends a bloom to the chain: the bright parts of the chain so far
// (or of opts.Src) are shrunk to a half and then a quarter of the screen size, blurred there
// and added back, so bright pixels glow. It declares three targets (half,
// quarter and a quarter-size scratch image) and appends five passes that run
// the engine's own shaders, which need no asset and are ready on the first
// frame.
//
// Passes added later see the bloom; passes added earlier are what it glows.
// Do not remove or reorder passes before the bloom's: the returned Bloom
// finds its passes by index. Use [Bloom.Set] to retune and [Bloom.SetOff] to
// switch the effect off.
func (p *PostChain) AddBloom(opts BloomOptions) Bloom {
	half := p.AddTarget(Target{Scale: 2})
	quarter := p.AddTarget(Target{Scale: 4})
	scratch := p.AddTarget(Target{Scale: 4})
	b := Bloom{first: len(p.Passes)}
	p.Passes = append(p.Passes,
		Pass{builtin: bloomBright, Out: half},
		Pass{builtin: bloomDown, Src: half, Out: quarter},
		Pass{builtin: bloomBlur, Src: quarter, Out: scratch},
		Pass{builtin: bloomBlur, Src: scratch, Out: quarter},
		Pass{builtin: bloomAdd, Extra: [3]TargetID{quarter}},
	)
	b.Set(p, opts)
	return b
}

// Set changes the options of the bloom; zero fields take their defaults.
func (b Bloom) Set(p *PostChain, opts BloomOptions) {
	o := opts.withDefaults()
	ps := p.Passes[b.first : b.first+bloomPasses]
	ps[0].Src = o.Src
	ps[0].Params = [4]float32{o.Threshold}
	ps[2].Params = [4]float32{o.Radius, 0}
	ps[3].Params = [4]float32{0, o.Radius}
	ps[4].Params = [4]float32{o.Strength}
}

// SetOff switches all passes of the bloom off or on.
func (b Bloom) SetOff(p *PostChain, off bool) {
	for i := range p.Passes[b.first : b.first+bloomPasses] {
		p.Passes[b.first+i].Off = off
	}
}
