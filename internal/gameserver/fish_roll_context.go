package gameserver

import (
	"hash/fnv"
	"strconv"
	"time"
)

// fishRollContext 只传递本次判鱼的区域和钩上饵，不改动存档格式。
type fishRollContext struct {
	x, z   float64
	hasPos bool
	// playerPosOnly 表示坐标来自人物，缺少本竿饵落点。
	playerPosOnly bool
	spot          uint64
	loc           string
	// uid 本次判鱼的玩家。窝料账本按 (uid, loc, 落点格子) 结算，
	// 没有它就查不到窝。
	uid        int64
	now        time.Time
	baits      []string
	lureBait   bool
	forRig     bool
	depthCM    int64
	lureLayer  float64
	lureAction string
	lureMask   uint32
	lureSpeed  int64
	weather    fishingWeather
	rigType    int64
	happyBite  float64
	onBoat     bool
	boatKnown  bool
	biteRoll   func() float64
	cache      *fishingRollCache
	skill      fishingSkillBonus
	spotEnv    fishSpotEnvironment
	runtime    *fishRollRuntime
	hook       gearPhysicsSpec
	query      *fishQueryCapture
	// gateBlocked 由 rollFishWithRules 挂指针：硬性鱼口门把本图名册剔空时置位，
	// 调用方据此跳过内置鱼池兜底——门拦下的杆就该空竿重排，不能从鱼池
	// 捞出不属于本图的鱼。指针随值拷贝存活。
	gateBlocked *bool
	// audit 把本次判鱼的位置与空因传回咬钩计时器。
	audit *fishRollAudit
	// pityEnv 是保底杂口标记：连续多轮空杆后由咬钩计时器置位，
	// 只放宽环境类因素（分布散布归零→0.15、水层地板≥0.05），
	// 食性硬门（G1 拟饵×非掠食鱼、明确禁饵）不放宽。
	pityEnv bool
}

type fishRollAudit struct {
	statLoc     string
	runtime     *fishRollRuntime
	emptyReason string
}

type fishRollRuntime struct {
	lure      lureAttributes
	lureKnown bool
	// levelChecked 保证一次判鱼只解析一次地点与海图区号。
	levelChecked bool
	// chartKnown 表示该地点有没有海图，zoneKnown 表示是否定位到声呐网格。
	chartKnown bool
	zone       int
	zoneKnown  bool
	// guideSpot 是已定位的纸图测深标注，0 表示未知。
	guideSpot int
	spotPref  map[string]float64
	spotData  bool
	// sonarSystemID 是海图声呐档案键（如 loc12_215）；内陆为空。
	sonarSystemID string
}

func (r *fishRollRuntime) chartZoneLabel() string {
	switch {
	case r == nil:
		return "未知"
	case !r.chartKnown:
		return "无海图"
	case !r.zoneKnown:
		return "未知"
	default:
		return strconv.Itoa(r.zone)
	}
}

func (r *fishRollRuntime) guideSpotLabel() string {
	if r == nil || r.guideSpot == 0 {
		if r != nil && r.zoneKnown {
			return "标注范围外"
		}
		return "未知"
	}
	return strconv.Itoa(r.guideSpot)
}

func rollContext(contexts []fishRollContext) fishRollContext {
	var c fishRollContext
	if len(contexts) > 0 {
		c = contexts[0]
	}
	if c.now.IsZero() {
		c.now = time.Now()
	}
	if c.cache == nil {
		c.cache = newFishingRollCache()
	}
	return c
}

func fishVariation(key string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return float64(h.Sum64()%10000) / 10000
}
