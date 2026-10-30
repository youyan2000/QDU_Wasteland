package hotdigest

import "strings"

// Language tracks. Three of the fields are split by the language of the
// material: Chinese, English, and "world" (French, German, Spanish, Japanese…).
const (
	TrackZh    = "zh"
	TrackEn    = "en"
	TrackWorld = "world"
)

// TrackOrder is the display order of the language tracks.
var TrackOrder = []string{TrackZh, TrackEn, TrackWorld}

// TrackTitle is the label shown for a language track.
func TrackTitle(track string) string {
	switch track {
	case TrackZh:
		return "华语"
	case TrackEn:
		return "英语"
	case TrackWorld:
		return "世界语言"
	default:
		return track
	}
}

// Source is one subscribed feed: an RSS/Atom site, or a peer-reviewed journal
// queried through Crossref.
type Source struct {
	ID    string
	Name  string
	Group string
	// Track is the language track inside a split field (empty elsewhere).
	Track string
	// Tier is "T1" (first party), "T1_5" (official-adjacent, standards body,
	// research lab) or "T2" (media, magazine, individual).
	Tier string
	// Lang is the editorial language of the source: "zh", "en", "fr", "de"…
	Lang string
	// Broad marks a general-interest feed that only contributes items whose
	// title or summary matches the group's keyword set.
	Broad bool
	// Feed is the RSS/Atom URL. Empty for journal sources.
	Feed string
	// ISSN selects a journal source: newest works are read from Crossref.
	ISSN string
	// Max caps how many items this source may contribute (0 = group default).
	Max int
}

// Kind reports whether the source produces news articles or journal papers.
func (s Source) Kind() string {
	if strings.TrimSpace(s.ISSN) != "" {
		return KindPaper
	}
	return KindNews
}

// tierWeight converts a source tier into the multiplier used by the heat score.
func tierWeight(tier string) float64 {
	switch tier {
	case "T1":
		return 1
	case "T1_5":
		return 0.82
	default:
		return 0.62
	}
}

// DefaultSources is the curated subscription list. Every RSS endpoint was
// verified reachable and every ISSN returned recent works from Crossref.
func DefaultSources() []Source {
	return []Source{
		// ── 哲学理论 · 英语 ─────────────────────────────────────────────────
		{ID: "j-mind", Name: "Mind", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0026-4423"},
		{ID: "j-phil-review", Name: "The Philosophical Review", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0031-8108"},
		{ID: "j-nous", Name: "Noûs", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0029-4624"},
		{ID: "j-ppr", Name: "Philosophy & Phenomenological Research", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0031-8205"},
		{ID: "j-analysis", Name: "Analysis", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0003-2638"},
		{ID: "j-phil-studies", Name: "Philosophical Studies", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0031-8116"},
		{ID: "j-ajp", Name: "Australasian Journal of Philosophy", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0004-8402"},
		{ID: "j-ejp", Name: "European Journal of Philosophy", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0966-8373"},
		{ID: "j-ethics", Name: "Ethics", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0014-1704"},
		{ID: "j-ppa", Name: "Philosophy & Public Affairs", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0048-3915"},
		{ID: "j-ergo", Name: "Ergo", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "2330-4014"},
		{ID: "j-philosophers-imprint", Name: "Philosophers' Imprint", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "1533-628X"},
		{ID: "j-inquiry", Name: "Inquiry", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0020-174X"},
		{ID: "j-japa", Name: "Journal of the American Philosophical Association", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "2053-4477"},
		{ID: "j-ratio", Name: "Ratio", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0034-0006"},
		{ID: "j-phil-quarterly", Name: "The Philosophical Quarterly", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0031-8094"},
		{ID: "j-cjp", Name: "Canadian Journal of Philosophy", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0045-5091"},
		{ID: "j-dialogue", Name: "Dialogue", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0012-2173"},
		{ID: "j-theoria", Name: "Theoria", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0040-5825"},
		{ID: "j-metaphilosophy", Name: "Metaphilosophy", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0026-1068"},
		{ID: "j-moral-phil", Name: "Journal of Moral Philosophy", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "1740-4681"},
		{ID: "j-political-theory", Name: "Political Theory", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0090-5917"},
		{ID: "j-phil-science", Name: "Philosophy of Science", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1", Lang: "en", ISSN: "0031-8248"},
		{ID: "j-monist", Name: "The Monist", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "0026-9662"},
		{ID: "j-ethics-journal", Name: "The Journal of Ethics", Group: GroupPhilosophy, Track: TrackEn, Tier: "T1_5", Lang: "en", ISSN: "1382-4554"},

		// ── 哲学理论 · 世界语言（法/德/西）──────────────────────────────────
		{ID: "j-revue-philosophique", Name: "Revue philosophique de la France et de l'étranger", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1", Lang: "fr", ISSN: "0035-3833"},
		{ID: "j-etudes-philosophiques", Name: "Les Études philosophiques", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1", Lang: "fr", ISSN: "0014-2166"},
		{ID: "j-archives-philosophie", Name: "Archives de Philosophie", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1_5", Lang: "fr", ISSN: "0003-9632"},
		{ID: "j-revue-metaphysique", Name: "Revue de métaphysique et de morale", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1", Lang: "fr", ISSN: "0035-1571"},
		{ID: "j-kant-studien", Name: "Kant-Studien", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1", Lang: "de", ISSN: "0022-8877"},
		{ID: "j-z-phil-forschung", Name: "Zeitschrift für philosophische Forschung", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1", Lang: "de", ISSN: "0044-3301"},
		{ID: "j-dt-z-philosophie", Name: "Deutsche Zeitschrift für Philosophie", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1_5", Lang: "de", ISSN: "0012-0936"},
		{ID: "j-revista-filosofia", Name: "Revista de Filosofía", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1_5", Lang: "es", ISSN: "0034-8244"},
		{ID: "j-critica", Name: "Crítica", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1_5", Lang: "es", ISSN: "0011-1503"},
		{ID: "j-dianoia", Name: "Dianoia", Group: GroupPhilosophy, Track: TrackWorld, Tier: "T1_5", Lang: "es", ISSN: "0185-2450"},

		// ── 哲学理论 · 华语：中文哲学期刊没有公开 RSS，暂用中文思想评论代
		{ID: "zh-initium-phil", Name: "端传媒", Group: GroupPhilosophy, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://theinitium.com/feed"},
		{ID: "zh-nyt-phil", Name: "纽约时报中文网", Group: GroupPhilosophy, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://cn.nytimes.com/rss/"},
		{ID: "zh-crossing-phil", Name: "换日线", Group: GroupPhilosophy, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://crossing.cw.com.tw/rss"},

		// ── 思想散文 · 英语 ─────────────────────────────────────────────────
		{ID: "aeon", Name: "Aeon", Group: GroupEssay, Track: TrackEn, Tier: "T1_5", Lang: "en", Feed: "https://aeon.co/feed.rss"},
		{ID: "noema", Name: "NOEMA", Group: GroupEssay, Track: TrackEn, Tier: "T1_5", Lang: "en", Feed: "https://www.noemamag.com/feed/"},
		{ID: "the-point", Name: "The Point", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://thepointmag.com/feed/"},
		{ID: "3am", Name: "3:AM Magazine", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://www.3ammagazine.com/3am/feed/"},
		{ID: "marginalian", Name: "The Marginalian", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://www.themarginalian.org/feed/"},
		{ID: "lithub", Name: "Literary Hub", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://lithub.com/feed/"},
		{ID: "longreads", Name: "Longreads", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://longreads.com/feed/"},
		{ID: "aldaily", Name: "Arts & Letters Daily", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://www.aldaily.com/feed/"},
		{ID: "big-think", Name: "Big Think", Group: GroupEssay, Track: TrackEn, Tier: "T2", Lang: "en", Broad: true, Feed: "https://bigthink.com/feed/"},

		// ── 思想散文 · 世界语言 ─────────────────────────────────────────────
		{ID: "lemonde-idees", Name: "Le Monde · Idées", Group: GroupEssay, Track: TrackWorld, Tier: "T1_5", Lang: "fr", Feed: "https://www.lemonde.fr/idees/rss_full.xml"},
		{ID: "merkur", Name: "Merkur", Group: GroupEssay, Track: TrackWorld, Tier: "T2", Lang: "de", Feed: "https://www.merkur-zeitschrift.de/feed/"},
		{ID: "letras-libres", Name: "Letras Libres", Group: GroupEssay, Track: TrackWorld, Tier: "T2", Lang: "es", Feed: "https://letraslibres.com/feed/"},
		{ID: "revista-de-libros", Name: "Revista de Libros", Group: GroupEssay, Track: TrackWorld, Tier: "T2", Lang: "es", Feed: "https://www.revistadelibros.com/feed/"},

		// ── 思想散文 · 华语：这三家是新闻站，只有命中散文关键词的条目才算 ──
		{ID: "zh-initium-essay", Name: "端传媒", Group: GroupEssay, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://theinitium.com/feed"},
		{ID: "zh-nyt-essay", Name: "纽约时报中文网", Group: GroupEssay, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://cn.nytimes.com/rss/"},
		{ID: "zh-crossing-essay", Name: "换日线", Group: GroupEssay, Track: TrackZh, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://crossing.cw.com.tw/rss"},

		// ── 当代诗歌 · 英语 ─────────────────────────────────────────────────
		{ID: "poetry-foundation", Name: "Poetry Foundation", Group: GroupPoetry, Track: TrackEn, Tier: "T1", Lang: "en", Feed: "http://feeds.feedburner.com/poetryfoundation"},
		{ID: "poetry-society", Name: "The Poetry Society", Group: GroupPoetry, Track: TrackEn, Tier: "T1_5", Lang: "en", Feed: "https://www.poetrysociety.org.uk/feed/"},
		{ID: "poetry-northwest", Name: "Poetry Northwest", Group: GroupPoetry, Track: TrackEn, Tier: "T2", Lang: "en", Feed: "https://www.poetrynw.org/feed/"},

		// ── 当代诗歌 · 世界语言 ─────────────────────────────────────────────
		{ID: "circulo-poesia", Name: "Círculo de Poesía", Group: GroupPoetry, Track: TrackWorld, Tier: "T2", Lang: "es", Feed: "https://circulodepoesia.com/feed/"},

		// ── 电力电子：产业媒体 + IEEE 期刊 ──────────────────────────────────
		{ID: "power-electronics-news", Name: "Power Electronics News", Group: GroupPower, Tier: "T1_5", Lang: "en", Feed: "https://www.powerelectronicsnews.com/feed/"},
		{ID: "ieee-spectrum-energy", Name: "IEEE Spectrum · Energy", Group: GroupPower, Tier: "T1_5", Lang: "en", Broad: true, Feed: "https://spectrum.ieee.org/feeds/topic/energy.rss"},
		{ID: "ieee-spectrum-semi", Name: "IEEE Spectrum · Semiconductors", Group: GroupPower, Tier: "T1_5", Lang: "en", Broad: true, Feed: "https://spectrum.ieee.org/feeds/topic/semiconductors.rss"},
		{ID: "edn", Name: "EDN", Group: GroupPower, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.edn.com/feed/"},
		{ID: "eetimes", Name: "EE Times", Group: GroupPower, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.eetimes.com/feed/"},
		{ID: "eejournal", Name: "EEJournal", Group: GroupPower, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.eejournal.com/feed/"},
		{ID: "eenews-europe", Name: "eeNews Europe", Group: GroupPower, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.eenewseurope.com/en/feed/"},
		{ID: "semiengineering", Name: "Semiconductor Engineering", Group: GroupPower, Tier: "T1_5", Lang: "en", Broad: true, Feed: "https://semiengineering.com/feed/"},
		{ID: "j-tpel", Name: "IEEE Trans. Power Electronics", Group: GroupPower, Tier: "T1", Lang: "en", ISSN: "0885-8993"},
		{ID: "j-tie", Name: "IEEE Trans. Industrial Electronics", Group: GroupPower, Tier: "T1", Lang: "en", ISSN: "0278-0046"},
		{ID: "j-jestpe", Name: "IEEE J. Emerging & Selected Topics in Power Electronics", Group: GroupPower, Tier: "T1", Lang: "en", ISSN: "2168-6777"},
		{ID: "j-tpwrs", Name: "IEEE Trans. Power Systems", Group: GroupPower, Tier: "T1", Lang: "en", ISSN: "0885-8950"},
		{ID: "j-tte", Name: "IEEE Trans. Transportation Electrification", Group: GroupPower, Tier: "T1", Lang: "en", ISSN: "2332-7782"},

		// ── 嵌入式：社区与厂商 + 系统/电路期刊 ──────────────────────────────
		{ID: "cnx-software", Name: "CNX Software", Group: GroupEmbedded, Tier: "T1_5", Lang: "en", Feed: "https://www.cnx-software.com/feed/"},
		{ID: "hackaday", Name: "Hackaday", Group: GroupEmbedded, Tier: "T2", Lang: "en", Feed: "https://hackaday.com/feed/"},
		{ID: "st-blog", Name: "ST Blog", Group: GroupEmbedded, Tier: "T1", Lang: "en", Feed: "https://blog.st.com/feed/"},
		{ID: "arduino-blog", Name: "Arduino Blog", Group: GroupEmbedded, Tier: "T1", Lang: "en", Feed: "https://blog.arduino.cc/feed/"},
		{ID: "raspberry-pi", Name: "Raspberry Pi News", Group: GroupEmbedded, Tier: "T1", Lang: "en", Feed: "https://www.raspberrypi.com/news/feed/"},
		{ID: "riscv", Name: "RISC-V International", Group: GroupEmbedded, Tier: "T1_5", Lang: "en", Feed: "https://riscv.org/feed/"},
		{ID: "linuxgizmos", Name: "LinuxGizmos", Group: GroupEmbedded, Tier: "T1_5", Lang: "en", Feed: "https://linuxgizmos.com/feed/"},
		{ID: "embedded-com", Name: "Embedded.com", Group: GroupEmbedded, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.embedded.com/feed/"},
		{ID: "electronics-weekly", Name: "Electronics Weekly", Group: GroupEmbedded, Tier: "T2", Lang: "en", Broad: true, Feed: "https://www.electronicsweekly.com/feed/"},
		{ID: "j-tecs", Name: "IEEE Trans. Embedded Computing Systems", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "1539-9087"},
		{ID: "j-esl", Name: "IEEE Embedded Systems Letters", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "1943-0663"},
		{ID: "j-tvlsi", Name: "IEEE Trans. VLSI Systems", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "1063-8210"},
		{ID: "j-jssc", Name: "IEEE J. Solid-State Circuits", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "0018-9200"},
		{ID: "j-tcas1", Name: "IEEE Trans. Circuits & Systems I", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "1549-8328"},
		{ID: "j-tcad", Name: "IEEE Trans. Computer-Aided Design", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "0278-0070"},
		{ID: "j-taco", Name: "ACM Trans. Architecture & Code Optimization", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "1544-3566"},
		{ID: "j-iotj", Name: "IEEE Internet of Things Journal", Group: GroupEmbedded, Tier: "T1", Lang: "en", ISSN: "2327-4662", Max: 3},

		// ── AI ─────────────────────────────────────────────────────────────
		{ID: "openai", Name: "OpenAI News", Group: GroupAI, Tier: "T1", Lang: "en", Feed: "https://openai.com/news/rss.xml"},
		{ID: "deepmind", Name: "Google DeepMind", Group: GroupAI, Tier: "T1", Lang: "en", Feed: "https://deepmind.google/blog/rss.xml"},
		{ID: "google-research", Name: "Google Research", Group: GroupAI, Tier: "T1", Lang: "en", Feed: "https://research.google/blog/rss/"},
		{ID: "huggingface", Name: "Hugging Face", Group: GroupAI, Tier: "T1", Lang: "en", Feed: "https://huggingface.co/blog/feed.xml"},
		{ID: "mistral", Name: "Mistral AI", Group: GroupAI, Tier: "T1", Lang: "en", Feed: "https://mistral.ai/news/rss"},
		{ID: "nvidia", Name: "NVIDIA Blog", Group: GroupAI, Tier: "T1", Lang: "en", Broad: true, Feed: "https://blogs.nvidia.com/feed/"},
		{ID: "msr", Name: "Microsoft Research", Group: GroupAI, Tier: "T1", Lang: "en", Broad: true, Feed: "https://www.microsoft.com/en-us/research/feed/"},
		{ID: "bair", Name: "Berkeley AI Research", Group: GroupAI, Tier: "T1_5", Lang: "en", Feed: "https://bair.berkeley.edu/blog/feed.xml"},
		{ID: "arxiv-cs-ai", Name: "arXiv cs.AI", Group: GroupAI, Tier: "T1_5", Lang: "en", Max: 4, Feed: "https://rss.arxiv.org/rss/cs.AI"},
		{ID: "mit-news-ai", Name: "MIT News · AI", Group: GroupAI, Tier: "T1_5", Lang: "en", Feed: "https://news.mit.edu/rss/topic/artificial-intelligence2"},
		{ID: "the-decoder", Name: "The Decoder", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://the-decoder.com/feed/"},
		{ID: "mit-tr-ai", Name: "MIT Technology Review · AI", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://www.technologyreview.com/topic/artificial-intelligence/feed"},
		{ID: "ars-ai", Name: "Ars Technica · AI", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://arstechnica.com/ai/feed/"},
		{ID: "tc-ai", Name: "TechCrunch · AI", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://techcrunch.com/category/artificial-intelligence/feed/"},
		{ID: "simon-willison", Name: "Simon Willison", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://simonwillison.net/atom/everything/"},
		{ID: "the-gradient", Name: "The Gradient", Group: GroupAI, Tier: "T2", Lang: "en", Feed: "https://thegradient.pub/rss/"},
		{ID: "qbitai", Name: "量子位", Group: GroupAI, Tier: "T1_5", Lang: "zh", Feed: "https://www.qbitai.com/feed"},
		{ID: "solidot", Name: "奇客 Solidot", Group: GroupAI, Tier: "T2", Lang: "zh", Broad: true, Feed: "https://www.solidot.org/index.rss"},

		// ── 潮流音乐 ───────────────────────────────────────────────────────
		{ID: "pitchfork", Name: "Pitchfork", Group: GroupMusic, Tier: "T1_5", Lang: "en", Feed: "https://pitchfork.com/rss/news/"},
		{ID: "fact", Name: "FACT Magazine", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.factmag.com/feed/"},
		{ID: "quietus", Name: "The Quietus", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://thequietus.com/feed"},
		{ID: "musicradar", Name: "MusicRadar", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.musicradar.com/rss"},
		{ID: "nme", Name: "NME", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.nme.com/feed"},
		{ID: "consequence", Name: "Consequence", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://consequence.net/feed/"},
		{ID: "brooklynvegan", Name: "BrooklynVegan", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.brooklynvegan.com/feed/"},
		{ID: "spin", Name: "Spin", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.spin.com/feed/"},
		{ID: "rollingstone-music", Name: "Rolling Stone · Music", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.rollingstone.com/music/feed/"},
		{ID: "lineofbestfit", Name: "The Line of Best Fit", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.thelineofbestfit.com/feed"},
		{ID: "xlr8r", Name: "XLR8R", Group: GroupMusic, Tier: "T2", Lang: "en", Feed: "https://www.xlr8r.com/feed/"},
	}
}

// groupKeywords is the vocabulary used for two jobs: a Broad source must match
// one of them to count, and each match adds a little heat.
var groupKeywords = map[string][]string{
	GroupPhilosophy: {
		"philosoph", "ethic", "metaphysic", "epistemolog", "phenomenolog",
		"ontolog", "dialectic", "hermeneutic", "aesthetic", "normative",
		"consciousness", "moral", "political theory", "semantic",
		"philosophie", "philosophisch", "filosof", "ética", "éthique",
		"哲学", "思想", "伦理", "认识论", "本体论", "辩证法", "现象学",
		"形而上学", "政治哲学", "观念史", "思想史",
	},
	GroupEssay: {
		"essay", "memory", "attention", "loneliness", "friendship", "grief",
		"meaning", "wonder", "silence", "walking", "reading", "writing",
		"love", "death", "time", "self", "art", "beauty", "story",
		"essai", "ensayo",
		"随笔", "散文", "专栏", "书评", "文化", "思想", "人文", "读书",
		"阅读", "写作", "记忆", "孤独", "意义", "访谈", "长文", "观点", "评论",
	},
	GroupPoetry: {
		"poem", "poet", "poetry", "verse", "stanza", "sonnet", "lyric",
		"poème", "poésie", "gedicht", "lyrik", "poema", "poesía", "verso",
		"诗歌", "诗", "诗人", "诗集", "读诗", "现代诗",
	},
	GroupPower: {
		"power electronic", "converter", "inverter", "rectifier", "dc-dc",
		"dcdc", "dc/ac", "ac/dc", "sic", "gan", "igbt", "mosfet",
		"wide bandgap", "wide-bandgap", "switching", "topology", "topologies",
		"llc", "pfc", "bidirectional", "transformer", "inductor", "capacitor",
		"motor drive", "grid", "ev charging", "charger", "wireless power",
		"on-board charger", "obc", "pwm", "resonant", "zvs", "zcs", "modulation",
		"电源", "变换器", "逆变", "整流", "拓扑", "变压器", "电感", "电容",
		"功率器件", "充电", "储能", "并网", "变流",
	},
	GroupEmbedded: {
		"embedded", "mcu", "microcontroller", "stm32", "esp32", "esp8266",
		"rp2040", "risc-v", "riscv", "firmware", "rtos", "zephyr", "freertos",
		"arduino", "raspberry pi", "fpga", "pcb", "kicad", "sensor", "i2c",
		"spi", "uart", "can bus", "modbus", "jtag", "swd", "system on module",
		"sbc", "single board", "dev board", "development board", "soc",
		"accelerator", "low power", "in-memory", "pim", "neuromorphic",
		"单片机", "嵌入式", "固件", "开发板", "芯片", "传感器", "实时操作系统",
		"物联网", "加速器",
	},
	GroupAI: {
		"llm", "language model", "transformer", "diffusion", "agent", "rag",
		"fine-tun", "inference", "gpu", "cuda", "neural", "deep learning",
		"machine learning", "benchmark", "multimodal", "reasoning", "embedding",
		"reinforcement", "model", "dataset", "open-weight", "quantization",
		"大模型", "人工智能", "机器学习", "深度学习", "推理", "训练", "智能体",
		"多模态", "思维链", "开源模型", "算力",
	},
	GroupMusic: {
		"album", "ep ", "single", "tour", "festival", "remix", "producer",
		"dj", "synth", "modular", "pedal", "vinyl", "label", "band",
		"artist", "song", "track", "mix", "live", "record",
		"专辑", "巡演", "音乐节", "混音", "合成器", "黑胶", "乐队", "单曲",
	},
}

// noiseWords mark marketing or off-topic listings that would otherwise use up a
// section's quota.
var noiseWords = []string{
	"sponsored", "advertisement", "webinar", "giveaway", "promo code",
	"discount code", "coupon", "buy now", "black friday", "cyber monday",
	"招聘", "诚聘", "优惠券", "限时优惠", "抽奖", "广告",
}

// looksLikeNoise reports whether a title reads as marketing rather than news.
func looksLikeNoise(title string) bool {
	lower := strings.ToLower(title)
	for _, w := range noiseWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// journalBookkeeping are the records every journal deposits alongside its
// articles (issue furniture). They are not reading material.
var journalBookkeeping = map[string]bool{
	"issue information":                   true,
	"issue information - editorial board": true,
	"table of contents":                   true,
	"contents":                            true,
	"front matter":                        true,
	"back matter":                         true,
	"editorial board":                     true,
	"masthead":                            true,
	"in this issue":                       true,
	"in this issue:":                      true,
	"cover image":                         true,
	"correction":                          true,
	"corrigendum":                         true,
	"erratum":                             true,
	"retraction":                          true,
	"call for papers":                     true,
	"prelims":                             true,
	"index":                               true,
	"author index":                        true,
	"subject index":                       true,
	"volume index":                        true,
	"annual index":                        true,
	"list of reviewers":                   true,
	"reviewers":                           true,
	"notes on contributors":               true,
	"about the contributors":              true,
	"books received":                      true,
	"referees":                            true,
	"editor's note":                       true,
	"editors' note":                       true,
	"editor's introduction":               true,
	"editors' introduction":               true,
	"guest editorial":                     true,
	"editorial":                           true,
	"preface":                             true,
	"erratum:":                            true,
}

// bookkeepingPrefixes catch records that carry an article title after a colon,
// such as "Editor's Note: Testing abductions from uncertain evidence".
var bookkeepingPrefixes = []string{
	"issue information",
	"editor's note",
	"editors' note",
	"editor's introduction",
	"editors' introduction",
	"correction:",
	"corrigendum:",
	"erratum:",
	"retraction:",
	"retraction—",
	"call for papers",
}

// isBookkeeping reports whether a journal record is issue furniture rather
// than an article.
func isBookkeeping(title string) bool {
	lower := strings.ToLower(strings.TrimSpace(title))
	// Crossref uses typographic apostrophes in several of these titles.
	lower = strings.ReplaceAll(lower, "\u2019", "'")
	lower = strings.ReplaceAll(lower, "\u2018", "'")
	lower = strings.TrimSuffix(lower, ".")
	if journalBookkeeping[lower] {
		return true
	}
	for _, prefix := range bookkeepingPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// lowValueWords mark recurring series that carry little new information.
var lowValueWords = []string{
	"podcast", "newsletter", "weekly roundup", "this week in", "edition #",
	"issue #", "lit hub daily", "morning news", "daily briefing", "ticker",
	"播客", "早报", "日报速览",
}

// looksLowValue reports whether an entry is a recurring series rather than a
// report of something that happened.
func looksLowValue(title string) bool {
	lower := strings.ToLower(title)
	for _, w := range lowValueWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// keywordHits counts how many of a group's keywords appear in the text.
func keywordHits(group, text string) int {
	lower := strings.ToLower(text)
	hits := 0
	seen := make(map[string]bool)
	for _, w := range groupKeywords[group] {
		w = strings.ToLower(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		if containsKeyword(lower, w) {
			hits++
		}
	}
	return hits
}

// onTopic reports whether a Broad source's item belongs to the group.
func onTopic(group, text string) bool { return keywordHits(group, text) > 0 }

// containsKeyword matches long terms as substrings ("philosoph" catches
// philosophy and philosophical) but requires a word boundary for short tokens,
// so "gan" does not fire inside "organ" and "sic" does not fire inside "music".
func containsKeyword(lower, term string) bool {
	if !needsBoundary(term) {
		return strings.Contains(lower, term)
	}
	for idx := 0; ; {
		p := strings.Index(lower[idx:], term)
		if p < 0 {
			return false
		}
		p += idx
		end := p + len(term)
		startOK := p == 0 || !isWordByte(lower[p-1])
		endOK := end == len(lower) || !isWordByte(lower[end])
		if startOK && endOK {
			return true
		}
		idx = p + 1
	}
}

func needsBoundary(term string) bool {
	if len(term) > 4 {
		return false
	}
	for i := 0; i < len(term); i++ {
		c := term[i]
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') {
			return false
		}
	}
	return len(term) > 0
}

func isWordByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
