package server

type HomePayload struct {
	Hero        HeroBook      `json:"hero"`
	Recommended []BookCard    `json:"recommended"`
	Updates     []UpdateEntry `json:"updates"`
}

type HeroBook struct {
	Label       string   `json:"label"`
	Title       string   `json:"title"`
	Meta        string   `json:"meta"`
	Description []string `json:"description"`
}

type BookCard struct {
	Title    string `json:"title"`
	Author   string `json:"author"`
	Category string `json:"category"`
	Variant  string `json:"variant"`
}

type UpdateEntry struct {
	Title   string `json:"title"`
	Chapter string `json:"chapter"`
	Time    string `json:"time"`
}

func buildHomePayload() HomePayload {
	return HomePayload{
		Hero: HeroBook{
			Label: "本周推荐",
			Title: "万古神帝",
			Meta:  "飞天鱼 · 玄幻 · 连载中",
			Description: []string{
				"八百年前，明帝之子张若尘，被他的未婚妻池瑶公主",
				"和大魔神王联手害死，一缕残魂被封印在昆仑界的",
				"无名神墓中。八百年后，张若尘从神墓中复活，重",
				"新踏上修行之路...",
			},
		},
		Recommended: []BookCard{
			{Title: "万古神帝", Author: "飞天鱼", Category: "玄幻", Variant: "hero"},
			{Title: "伏天氏", Author: "净无痕", Category: "玄幻", Variant: "paper"},
			{Title: "剑来", Author: "烽火戏诸侯", Category: "仙侠", Variant: "sword"},
			{Title: "大道争锋", Author: "误道者", Category: "仙侠", Variant: "ink"},
			{Title: "诡秘之主", Author: "爱潜水的乌贼", Category: "玄幻", Variant: "mystery"},
			{Title: "夜的命名术", Author: "会说话的肘子", Category: "都市", Variant: "night"},
			{Title: "大奉打更人", Author: "卖报小郎君", Category: "都市", Variant: "paper"},
			{Title: "我师兄实在太稳健了", Author: "言归正传", Category: "仙侠", Variant: "ink"},
		},
		Updates: []UpdateEntry{
			{Title: "从红月开始", Chapter: "第九百六十六章：新的征程", Time: "2 小时前"},
			{Title: "玩家凶猛", Chapter: "第 2248 章 沉默的公牛（求月票）", Time: "3 小时前"},
			{Title: "轮回乐园", Chapter: "第 4110 章 最后的对峙", Time: "4 小时前"},
			{Title: "第一序列", Chapter: "第 2720 章：“希望”", Time: "5 小时前"},
			{Title: "没钱修什么仙？", Chapter: "第 197 章：你还真别说", Time: "6 小时前"},
			{Title: "这游戏也太真实了", Chapter: "第 1234 章：新的开始", Time: "8 小时前"},
		},
	}
}
