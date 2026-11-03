package worker

import (
	"math/rand/v2"
	"strings"
)

var zhNames = []string{
	"曹操", "刘备", "孙权", "诸葛亮", "关羽", "张飞", "赵云", "马超", "黄忠", "魏延",
	"姜维", "邓艾", "钟会", "司马懿", "司马昭", "司马师", "夏侯惇", "夏侯渊", "曹仁", "曹洪",
	"曹纯", "曹休", "曹真", "张辽", "乐进", "于禁", "徐晃", "张郃", "许褚", "典韦",
	"荀彧", "荀攸", "贾诩", "郭嘉", "程昱", "刘晔", "蒋济", "华歆", "王朗", "董昭",
	"满宠", "毛玠", "崔琰", "陈群", "司马朗", "钟繇", "王粲", "杨修", "孔融", "陈琳",
	"曹丕", "曹植", "曹彰", "曹冲", "许攸", "审配", "逢纪", "郭图", "田丰", "沮授",
	"颜良", "文丑", "高览", "袁绍", "袁术", "袁谭", "袁尚", "袁熙", "刘表", "蔡瑁",
	"蒯越", "刘琦", "刘琮", "黄祖", "甘宁", "凌统", "周瑜", "鲁肃", "吕蒙", "陆逊",
	"陆抗", "程普", "黄盖", "韩当", "蒋钦", "周泰", "陈武", "潘璋", "丁奉", "徐盛",
	"朱然", "孙策", "孙坚", "孙翊", "孙皎", "孙亮", "孙休", "孙皓", "张昭", "张纮",
	"顾雍", "诸葛瑾", "步骘", "吾粲", "骆统", "虞翻", "陆绩", "张温", "严畯", "薛综",
	"程秉", "周鲂", "是仪", "吕范", "朱桓", "全琮", "吕岱", "孙韶", "诸葛恪", "孙峻",
	"孙綝", "濮阳兴", "张布", "万彧", "刘禅", "关兴", "关平", "张苞", "马谡", "王平",
	"廖化", "向宠", "蒋琬", "费祎", "董允", "陈祗", "吕乂", "张翼", "柳隐", "罗宪",
	"霍峻", "霍弋", "邓芝", "樊建", "宗预", "辅匡", "孙干", "简雍", "麋竺", "麋芳",
	"貂蝉", "王允", "吕布", "陈宫", "高顺", "宋宪", "魏续", "侯成", "张邈", "陈登",
	"陶谦", "曹嵩", "丁原", "何进", "董卓", "李傕", "郭汜", "皇甫嵩", "朱儁", "卢植",
	"刘虞", "公孙瓒", "张燕", "张绣", "张角", "张宝", "张梁", "管亥", "波才", "程远志",
	"华雄", "吕公", "纪灵", "桥玄", "何颙", "蒯良", "张肃", "刘繇", "许劭", "郑玄",
	"蔡邕", "王越", "史阿", "胡轸", "吕旷", "吕翔", "眭固", "韩浩", "史涣", "韩遂",
}

var enNames = []string{
	"Newton", "Darwin", "Einstein", "Tesla", "Curie", "Faraday", "Galileo", "Copernicus", "Kepler", "Brahe",
	"Maxwell", "Planck", "Bohr", "Heisenberg", "Schrodinger", "Feynman", "Dirac", "Rutherford", "Thomson", "Chadwick",
	"Mendeleev", "Lavoisier", "Priestley", "Dalton", "Avogadro", "Boltzmann", "Carnot", "Joule", "Kelvin", "Rankine",
	"Euler", "Gauss", "Riemann", "Cauchy", "Fourier", "Laplace", "Lagrange", "Pascal", "Fermat", "Archimedes",
	"Pythagoras", "Euclid", "Eratosthenes", "Hipparchus", "Ptolemy", "Thales", "Democritus", "Aristotle", "Bacon", "Descartes",
	"Hooke", "Boyle", "Huygens", "Columbus", "Vespucci", "Magellan", "Drake", "Cook", "Cabot", "Polo",
	"Mendel", "Lamarck", "Linnaeus", "Buffon", "Cuvier", "Huxley", "Haeckel", "Pasteur", "Koch", "Lister",
	"Fleming", "Jenner", "Harvey", "Vesalius", "Hippocrates", "Watt", "Stephenson", "Edison", "Marconi", "Bell",
	"Morse", "Babbage", "Lovelace", "Turing", "Hubble", "Sagan", "Hawking", "Penrose", "Dyson", "Oppenheimer",
	"Fermi", "Compton", "Ampere", "Volta", "Ohm", "Coulomb", "Hertz", "Lorentz", "Mach", "Doppler",
	"Celsius", "Fahrenheit", "Clausius", "Helmholtz", "Kirchhoff", "Bunsen", "Liebig", "Kekule", "Herschel", "Cassini",
	"Halley", "Flamsteed", "Bradley", "Bessel", "Fraunhofer", "Huggins", "Adams", "Leverrier", "Roentgen", "Becquerel",
	"Meitner", "Hahn", "Szilard", "Teller", "Bethe", "Seaborg", "Nobel", "Benz", "Wright", "Goddard",
	"Braun", "Glenn", "Armstrong", "Watson", "Crick", "Franklin", "Wilkins", "McClintock", "Morgan", "Muller",
	"Beadle", "Tatum", "Avery", "Wegener", "Richter", "Lyell", "Hutton", "Agassiz", "Holmes", "Wilson",
	"Shannon", "Wiener", "Neumann", "Hopper", "Knuth", "Dijkstra", "Chomsky", "McCarthy", "Minsky", "Wirth",
	"Vinci", "Gutenberg", "Wren", "Brunel", "Newcomen", "Langley", "Lilienthal", "Zeppelin", "Wallace", "Henslow",
	"Gamow", "Milne", "Chandrasekhar", "Pauli", "Born", "Sommerfeld", "Moseley", "Soddy", "Aston", "Lemaitre",
	"Langmuir", "Millikan", "Michelson", "Morley", "Raman", "Bose", "Ramanujan", "Hardy", "Littlewood", "Eddington",
	"Poincare", "Hilbert", "Cantor", "Dedekind", "Peano", "Frege", "Russell", "Whitehead", "Godel", "Church",
}

func NamePool(lang string) []string {
	if lang == "zh" {
		return zhNames
	}
	return enNames
}

func PickRandomName(pool []string, used map[string]struct{}) (string, bool) {
	available := make([]string, 0, len(pool))
	usedLower := make(map[string]struct{})
	for k := range used {
		usedLower[strings.ToLower(k)] = struct{}{}
	}
	for _, name := range pool {
		if _, ok := usedLower[strings.ToLower(name)]; !ok {
			available = append(available, name)
		}
	}
	if len(available) == 0 {
		return "", false
	}
	return available[rand.IntN(len(available))], true
}
