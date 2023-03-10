package filter

import (
	"strings"

	"github.com/duo/octopus/internal/common"
)

var replacer = strings.NewReplacer(
	"[ๅพฎ็ฌ]", "๐", "[Smile]", "๐",
	"[ๆๅด]", "๐", "[Grimace]", "๐",
	"[่ฒ]", "๐", "[Drool]", "๐",
	"[ๅๅ]", "๐ณ", "[Scowl]", "๐ณ",
	"[ๅพๆ]", "๐", "[Chill]", "๐",
	"[ๆตๆณช]", "๐ญ", "[Sob]", "๐ญ",
	"[ๅฎณ็พ]", "โบ๏ธ", "[Shy]", "โบ๏ธ",
	"[้ญๅด]", "๐ค", "[Shutup]", "๐ค",
	"[็ก]", "๐ด", "[Sleep]", "๐ด",
	"[ๅคงๅญ]", "๐ฃ", "[Cry]", "๐ฃ",
	"[ๅฐดๅฐฌ]", "๐ฐ", "[Awkward]", "๐ฐ",
	"[ๅๆ]", "๐ก", "[Pout]", "๐ก",
	"[่ฐ็ฎ]", "๐", "[Wink]", "๐",
	"[ๅฒ็]", "๐", "[Grin]", "๐",
	"[ๆ่ฎถ]", "๐ฑ", "[Surprised]", "๐ฑ",
	"[้พ่ฟ]", "๐", "[Frown]", "๐",
	"[ๅง]", "โบ๏ธ", "[Tension]", "โบ๏ธ",
	"[ๆ็]", "๐ซ", "[Scream]", "๐ซ",
	"[ๅ]", "๐คข", "[Puke]", "๐คข",
	"[ๅท็ฌ]", "๐", "[Chuckle]", "๐",
	"[ๆๅฟซ]", "โบ๏ธ", "[Joyful]", "โบ๏ธ",
	"[็ฝ็ผ]", "๐", "[Slight]", "๐",
	"[ๅฒๆข]", "๐", "[Smug]", "๐",
	"[ๅฐ]", "๐ช", "[Drowsy]", "๐ช",
	"[ๆๆ]", "๐ฑ", "[Panic]", "๐ฑ",
	"[ๆตๆฑ]", "๐", "[Sweat]", "๐",
	"[ๆจ็ฌ]", "๐", "[Laugh]", "๐",
	"[ๆ ้ฒ]", "๐", "[Loafer]", "๐",
	"[ๅฅๆ]", "๐ช", "[Strive]", "๐ช",
	"[ๅ้ช]", "๐ค", "[Scold]", "๐ค",
	"[็้ฎ]", "โ", "[Doubt]", "โ",
	"[ๅ]", "๐ค", "[Shhh]", "๐ค",
	"[ๆ]", "๐ฒ", "[Dizzy]", "๐ฒ",
	"[่กฐ]", "๐ณ", "[BadLuck]", "๐ณ",
	"[้ชท้ซ]", "๐", "[Skull]", "๐",
	"[ๆฒๆ]", "๐", "[Hammer]", "๐",
	"[ๅ่ง]", "๐\u200dโ", "[Bye]", "๐\u200dโ",
	"[ๆฆๆฑ]", "๐ฅ", "[Relief]", "๐ฅ",
	"[ๆ ้ผป]", "๐คท\u200dโ", "[DigNose]", "๐คท\u200dโ",
	"[้ผๆ]", "๐", "[Clap]", "๐",
	"[ๅ็ฌ]", "๐ป", "[Trick]", "๐ป",
	"[ๅทฆๅผๅผ]", "๐พ", "[Bah๏ผL]", "๐พ",
	"[ๅณๅผๅผ]", "๐พ", "[Bah๏ผR]", "๐พ",
	"[ๅๆฌ ]", "๐ช", "[Yawn]", "๐ช",
	"[้่ง]", "๐", "[Lookdown]", "๐",
	"[ๅงๅฑ]", "๐ฃ", "[Wronged]", "๐ฃ",
	"[ๅฟซๅญไบ]", "๐", "[Puling]", "๐",
	"[้ด้ฉ]", "๐", "[Sly]", "๐",
	"[ไบฒไบฒ]", "๐", "[Kiss]", "๐",
	"[ๅฏๆ]", "๐ป", "[Whimper]", "๐ป",
	"[่ๅ]", "๐ช", "[Cleaver]", "๐ช",
	"[่ฅฟ็]", "๐", "[Melon]", "๐",
	"[ๅค้]", "๐บ", "[Beer]", "๐บ",
	"[ๅๅก]", "โ", "[Coffee]", "โ",
	"[็ชๅคด]", "๐ท", "[Pig]", "๐ท",
	"[็ซ็ฐ]", "๐น", "[Rose]", "๐น",
	"[ๅ่ฐข]", "๐ฅ", "[Wilt]", "๐ฅ",
	"[ๅดๅ]", "๐", "[Lip]", "๐",
	"[็ฑๅฟ]", "โค๏ธ", "[Heart]", "โค๏ธ",
	"[ๅฟ็ข]", "๐", "[BrokenHeart]", "๐",
	"[่็ณ]", "๐", "[Cake]", "๐",
	"[็ธๅผน]", "๐ฃ", "[Bomb]", "๐ฃ",
	"[ไพฟไพฟ]", "๐ฉ", "[Poop]", "๐ฉ",
	"[ๆไบฎ]", "๐", "[Moon]", "๐",
	"[ๅคช้ณ]", "๐", "[Sun]", "๐",
	"[ๆฅๆฑ]", "๐ค", "[Hug]", "๐ค",
	"[ๅผบ]", "๐", "[Strong]", "๐",
	"[ๅผฑ]", "๐", "[Weak]", "๐",
	"[ๆกๆ]", "๐ค", "[Shake]", "๐ค",
	"[่ๅฉ]", "โ๏ธ", "[Victory]", "โ๏ธ",
	"[ๆฑๆณ]", "๐", "[Salute]", "๐",
	"[ๅพๅผ]", "๐\u200dโ", "[Beckon]", "๐\u200dโ",
	"[ๆณๅคด]", "๐", "[Fist]", "๐",
	"[OK]", "๐",
	"[่ทณ่ทณ]", "๐", "[Waddle]", "๐",
	"[ๅๆ]", "๐", "[Tremble]", "๐",
	"[ๆ็ซ]", "๐ก", "[Aaagh!]", "๐ก",
	"[่ฝฌๅ]", "๐บ", "[Twirl]", "๐บ",
	"[ๅฟๅ]", "๐คฃ", "[Hey]", "๐คฃ",
	"[ๆ่ธ]", "๐คฆ\u200dโ", "[Facepalm]", "๐คฆ\u200dโ",
	"[ๅฅธ็ฌ]", "๐", "[Smirk]", "๐",
	"[ๆบๆบ]", "๐ค", "[Smart]", "๐ค",
	"[็ฑ็]", "๐", "[Concerned]", "๐",
	"[่ถ]", "โ๏ธ", "[Yeah!]", "โ๏ธ",
	"[็บขๅ]", "๐งง", "[Packet]", "๐งง",
	"[้ธก]", "๐ฅ", "[Chick]", "๐ฅ",
	"[่ก็]", "๐ฏ๏ธ", "[Candle]", "๐ฏ๏ธ",
	"[็ณๅคงไบ]", "๐ฅ",
	"[ThumbsUp]", "๐", "[ThumbsDown]", "๐",
	"[Peace]", "โ๏ธ",
	"[Pleased]", "๐",
	"[Rich]", "๐",
	"[Pup]", "๐ถ",
	"[ๅ็]", "๐\u200d๐", "[Onlooker]", "๐\u200d๐",
	"[ๅ ๆฒน]", "๐ช\u200d๐", "[GoForIt]", "๐ช\u200d๐",
	"[ๅ ๆฒนๅ ๆฒน]", "๐ช\u200d๐ท",
	"[ๆฑ]", "๐", "[Sweats]", "๐",
	"[ๅคฉๅ]", "๐ฑ", "[OMG]", "๐ฑ",
	"[Emm]", "๐ค",
	"[็คพไผ็คพไผ]", "๐", "[Respect]", "๐",
	"[ๆบๆด]", "๐ถ\u200d๐", "[Doge]", "๐ถ\u200d๐",
	"[ๅฅฝ็]", "๐\u200d๐", "[NoProb]", "๐\u200d๐",
	"[ๅ]", "๐คฉ", "[Wow]", "๐คฉ",
	"[ๆ่ธ]", "๐\u200d๐ค", "[MyBad]", "๐\u200d๐ค",
	"[็ ดๆถไธบ็ฌ]", "๐", "[็ ดๆถ็บ็ฌ]", "๐", "[Lol]", "๐",
	"[่ฆๆถฉ]", "๐ญ", "[Hurt]", "๐ญ",
	"[็ฟป็ฝ็ผ]", "๐", "[Boring]", "๐",
	"[่ฃๅผ]", "๐ซ ", "[Broken]", "๐ซ ",
	"[็็ซน]", "๐งจ", "[Firecracker]", "๐งจ",
	"[็่ฑ]", "๐", "[Fireworks]", "๐",
	"[็ฆ]", "๐งง", "[Blessing]", "๐งง",
	"[็คผ็ฉ]", "๐", "[Gift]", "๐",
	"[ๅบ็ฅ]", "๐", "[Party]", "๐",
	"[ๅๅ]", "๐", "[Worship]", "๐",
	"[ๅนๆฐ]", "๐ฎโ๐จ", "[Sigh]", "๐ฎโ๐จ",
	"[่ฎฉๆ็็]", "๐", "[LetMeSee]", "๐",
	"[666]", "6๏ธโฃ6๏ธโฃ6๏ธโฃ",
	"[ๆ ่ฏญ]", "๐", "[Duh]", "๐",
	"[ๅคฑๆ]", "๐", "[Let Down]", "๐",
	"[ๆๆง]", "๐จ", "[Terror]", "๐จ",
	"[่ธ็บข]", "๐ณ", "[Flushed]", "๐ณ",
	"[็็]", "๐ท", "[Sick]", "๐ท",
	"[็ฌ่ธ]", "๐", "[Happy]", "๐",
)

type EmoticonFilter struct {
}

// WeChat -> Telegram: replace WeChat eomtion
func (f EmoticonFilter) Process(in *common.OctopusEvent) *common.OctopusEvent {
	if in.Vendor.Type == "wechat" {
		in.Content = replacer.Replace(in.Content)
	}
	return in
}
