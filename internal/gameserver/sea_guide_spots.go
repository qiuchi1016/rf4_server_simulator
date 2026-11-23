package gameserver

// 挪威海攻略以纸地图的测深标注命名钓点。同一深度可有多处，zone 是
// 标注中心所在的声呐矩形，0 表示中心在矩形覆盖范围外。
// 4.0.25034 原图（2048²）像素换算为海床世界坐标：
// x=(u-78)/0.38，z=(1970-v)/0.38-1957。只存核对后的定位数值。
var seaGuideMarks = []struct {
	spot, zone int
	x, z       float64
}{
	{30, 125, 1384.21, 569.32},
	{30, 144, 1834.21, 319.32},
	{34, 198, 2342.11, -272.79},
	{41, 258, 1531.58, -1093.84},
	{55, 186, 3710.53, -25.42},
	{75, 95, 3921.05, 1182.47},
	{80, 252, 3281.58, -925.42},
	{100, 0, 3921.05, 2093},
	{120, 170, 4257.89, 6.16},
	{120, 231, 2128.95, -864.89},
	{120, 273, 2928.95, -1491.21},
	{215, 154, 4252.63, 335.11},
}

// seaGuideSpotAtRoll 只识别已对照海图和海床高度的中心 20 米范围。
// 海钓抛竿有时上报局部坐标；与已知声呐矩形冲突时，坑位保持未知。
func seaGuideSpotAtRoll(c *fishRollContext) int {
	if !c.hasPos || c.playerPosOnly || !c.runtime.chartKnown {
		return 0
	}
	const radius = 20.0
	for _, mark := range seaGuideMarks {
		dx, dz := c.x-mark.x, c.z-mark.z
		if dx*dx+dz*dz <= radius*radius {
			if c.runtime.zoneKnown {
				zone, ok := sonarZoneForPoint("level_012_north_sea", c.x, c.z)
				if !ok || zone != c.runtime.zone {
					return 0
				}
			}
			return mark.spot
		}
	}
	return 0
}
