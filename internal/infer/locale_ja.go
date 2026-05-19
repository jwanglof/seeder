package infer

import (
	"fmt"

	"github.com/brianvoe/gofakeit/v7"
)

var (
	jaLastNames = []string{
		"佐藤", "鈴木", "高橋", "田中", "伊藤", "渡辺", "山本", "中村", "小林", "加藤",
		"吉田", "山田", "佐々木", "山口", "斎藤", "松本", "井上", "木村", "林", "清水",
		"山崎", "森", "池田", "橋本", "阿部", "石川", "山下", "中島", "石井", "小川",
		"前田", "岡田", "長谷川", "藤田", "後藤", "近藤", "村上", "遠藤", "青木", "坂本",
		"斉藤", "福田", "太田", "西村", "藤井", "金子", "岡本", "中川", "中野", "原田",
		"小野", "田村", "竹内", "金井", "和田", "中山", "石田", "上田", "森田", "原",
		"柴田", "酒井", "工藤", "横山", "宮崎", "宮本", "内田", "高木", "安藤", "島田",
		"谷口", "大野", "高田", "丸山", "今井", "河野", "藤原", "村田", "武田", "上野",
		"杉山", "増田", "小島", "平野", "大塚", "千葉", "久保", "松井", "岩崎", "桜井",
		"野口", "松田", "木下", "野村", "菊地", "新井", "渡部", "野田", "田口", "小山",
	}
	jaFirstNames = []string{
		"陽翔", "湊", "蓮", "樹", "悠真", "蒼", "朝陽", "新", "陽斗", "颯太",
		"大翔", "大和", "悠人", "陽向", "結翔", "凛", "律", "暖", "奏", "翔",
		"陽菜", "結愛", "凜", "結菜", "葵", "美月", "杏", "陽葵", "心春", "莉子",
		"咲良", "結衣", "結月", "心愛", "ひなた", "心晴", "莉緒", "彩葉", "芽依", "凪",
		"翔太", "健太", "拓海", "翔", "海斗", "颯", "陸", "悠", "蒼空", "悠斗",
		"美咲", "あかり", "ひかり", "さくら", "ゆい", "あおい", "つむぎ", "みお", "ほのか", "つぐみ",
		"太郎", "次郎", "三郎", "一郎", "健", "誠", "学", "豊", "勇", "守",
		"花子", "幸子", "京子", "智子", "和子", "恵子", "良子", "弘子", "純子", "由美",
	}
	jaPrefectures = []string{
		"北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県",
		"茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県",
		"新潟県", "富山県", "石川県", "福井県", "山梨県", "長野県", "岐阜県",
		"静岡県", "愛知県", "三重県", "滋賀県", "京都府", "大阪府", "兵庫県",
		"奈良県", "和歌山県", "鳥取県", "島根県", "岡山県", "広島県", "山口県",
		"徳島県", "香川県", "愛媛県", "高知県", "福岡県", "佐賀県", "長崎県",
		"熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
	}
	jaCities = []string{
		"千代田区", "中央区", "港区", "新宿区", "渋谷区", "世田谷区", "目黒区", "品川区", "大田区", "杉並区",
		"豊島区", "練馬区", "板橋区", "北区", "足立区", "葛飾区", "江戸川区", "墨田区", "江東区", "台東区",
		"文京区", "中野区", "荒川区",
		"横浜市", "川崎市", "相模原市", "さいたま市", "千葉市", "船橋市",
		"札幌市", "仙台市", "新潟市", "金沢市", "静岡市", "浜松市", "名古屋市",
		"京都市", "大阪市", "堺市", "神戸市", "岡山市", "広島市",
		"北九州市", "福岡市", "熊本市", "鹿児島市", "那覇市",
	}
)

func lastNameJA(f *gofakeit.Faker) any  { return randomFrom(f, jaLastNames) }
func firstNameJA(f *gofakeit.Faker) any { return randomFrom(f, jaFirstNames) }

func fullNameJA(f *gofakeit.Faker) any {
	return fmt.Sprintf("%s %s", randomFrom(f, jaLastNames), randomFrom(f, jaFirstNames))
}

func prefectureJA(f *gofakeit.Faker) any { return randomFrom(f, jaPrefectures) }
func cityJA(f *gofakeit.Faker) any       { return randomFrom(f, jaCities) }
func countryJA(*gofakeit.Faker) any      { return "日本" }

// addressJA returns "{prefecture}{city}{chome}-{ban}-{go}" (e.g.,
// "東京都港区1-2-3"). The numeric tail mirrors gofakeit's compact street
// notation; we keep it ASCII so downstream collation behaves predictably.
func addressJA(f *gofakeit.Faker) any {
	chome := f.Number(1, 9)
	ban := f.Number(1, 30)
	go_ := f.Number(1, 30)
	return fmt.Sprintf("%s%s%d-%d-%d", randomFrom(f, jaPrefectures), randomFrom(f, jaCities), chome, ban, go_)
}

// zipJA returns a Japanese postal code in "NNN-NNNN" form.
func zipJA(f *gofakeit.Faker) any {
	return fmt.Sprintf("%03d-%04d", f.Number(0, 999), f.Number(0, 9999))
}

// phoneJA returns a mobile-style number ("0[789]0-NNNN-NNNN"). Fixed-line
// formats vary by prefecture, so we stick to mobile for predictability.
func phoneJA(f *gofakeit.Faker) any {
	prefixes := []string{"070", "080", "090"}
	return fmt.Sprintf("%s-%04d-%04d", prefixes[f.Number(0, len(prefixes)-1)], f.Number(0, 9999), f.Number(0, 9999))
}

