package gameserver

import "math"

const seaTerrainOriginZ = -1957

// 挪威海 4.0.25034 level24 的 WorldShifter（组件 51259）有五个预设偏移，
// 加上初始未平移的坐标系。14/2 的钓组落点随原点平移，同包的姿态使用地图相对坐标。
var seaFrameOffsets = [...][2]float64{
	{0, 0}, {248.9757233, 474.9511719}, {2342.3054199, -1039.1062012},
	{2891.2993164, 227.2830048}, {3928.8159180, 689.5770264}, {3640.3603516, -575.5109253},
}

func seaChartLanding(rawX, rawZ, poseX, poseZ, castM float64) (x, z float64, known bool) {
	if !finiteSonarNumber(castM) || castM < 0 || !finiteSonarNumber(poseX) || !finiteSonarNumber(poseZ) {
		return 0, 0, false
	}
	// 用同竿的抛距和姿态识别唯一可达的坐标系。2 米余量覆盖竿位与姿态原点差。
	// 最终参与判鱼的是钓组落点加偏移，姿态只用于选坐标系。
	for _, offset := range seaFrameOffsets {
		cx, cz := rawX+offset[0], rawZ+offset[1]
		if math.Hypot(cx-poseX, cz-(poseZ+seaTerrainOriginZ)) > castM+2 {
			continue
		}
		if known {
			return 0, 0, false
		}
		x, z, known = cx, cz, true
	}
	return x, z, known
}

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

func seaGuideSpotForPoint(x, z float64) int {
	u, v := 78+x*.38, 1970-(z-seaTerrainOriginZ)*.38
	for _, bank := range seaChartBanks {
		if bank.contains(u, v) {
			return bank.spot
		}
	}
	for _, mark := range seaGuideMarks {
		dx, dz := x-mark.x, z-mark.z
		if dx*dx+dz*dz <= seaGuideRadius*seaGuideRadius {
			return mark.spot
		}
	}
	return 0
}
