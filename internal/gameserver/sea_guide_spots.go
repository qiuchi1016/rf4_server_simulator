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

const seaGuideRadius = 20.0

// spot 用于渔获观测，只有实际落点才能命中。possible 用于区域限制：
// nil 表示没有定位证据，非 nil 的空表表示当前位置排除了所有已标定钓点。
func seaGuideSpotsAtRoll(c *fishRollContext) (spot int, possible []int) {
	if !c.runtime.chartKnown {
		return 0, nil
	}
	systemID := c.runtime.sonarSystemID
	if c.hasPos && !c.playerPosOnly {
		// 没有场景编号时直接使用落点。有编号时先排除局部坐标与世界坐标不一致的情况。
		consistent := true
		if systemID != "" {
			zone, ok := sonarZoneForPoint("level_012_north_sea", c.x, c.z)
			consistent = ok && c.runtime.zoneKnown && zone == c.runtime.zone
		}
		if consistent {
			if spot := seaGuideSpotForPoint(c.x, c.z); spot > 0 {
				return spot, []int{spot}
			}
			return 0, []int{}
		}
	}

	// 只有客户端声呐区号也能排除远处钓点。检查圆与矩形的交集，保留跨界钓点。
	grids, _ := loadSonarGrids()
	grid, ok := grids[systemID]
	if !ok {
		return 0, nil
	}
	possible = []int{}
	for _, mark := range seaGuideMarks {
		dx := mark.x - clampFloat(mark.x, grid.topLeftX, grid.topLeftX+grid.width)
		dz := mark.z - clampFloat(mark.z, grid.topLeftZ-grid.length, grid.topLeftZ)
		if dx*dx+dz*dz <= seaGuideRadius*seaGuideRadius {
			possible = append(possible, mark.spot)
		}
	}
	return 0, possible
}

func seaGuideSpotForPoint(x, z float64) int {
	for _, mark := range seaGuideMarks {
		dx, dz := x-mark.x, z-mark.z
		if dx*dx+dz*dz <= seaGuideRadius*seaGuideRadius {
			return mark.spot
		}
	}
	return 0
}
