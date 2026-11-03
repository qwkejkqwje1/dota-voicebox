package winapi

import "github.com/qwkejkqwje1/dota-voicebox/internal/overlay"

// TrayItem — пункт меню значка в трее.
type TrayItem struct {
	ID       string
	Title    string
	Checked  bool
	Disabled bool
	Default  bool // жирный пункт (то же, что щелчок по значку)
	Sep      bool // разделитель
	Sub      []TrayItem
}

// ShellOptions — значок в трее, быстрое меню и уведомления поверх игры.
type ShellOptions struct {
	Tooltip  string
	Menu     *overlay.Controller
	Wheel    *overlay.WheelCtl // колесо звуков (может быть nil)
	R        *overlay.Renderer
	TrayMenu func() []TrayItem // пункты при правом щелчке по значку
	OnTray   func(id string)   // выбран пункт; "open" — щелчок по значку
	// OnMenuMoved — быстрое меню перетащили мышью: новое место (доли свободного места, см. overlay.Place).
	OnMenuMoved func(x, y float64)
}
