# Пожар повреждённого здания

Спрайт: `internal/assets/generated/building_fire.png`, три равные клетки по горизонтали.
Кадры загружаются один раз в 64×64, огонь использует общий счётчик анимации.
При 99 ХП — один небольшой очаг; при низких ХП — до четырёх крупных очагов.
Огонь исчезает при полном ремонте или разрушении. На поле фермы огонь не распространяется.
Эффект не меняет боевой урон и не создаёт сущности.

Источник: встроенный imagegen, 2026-09-07. Промпт:
Transparent pixel-art sprite sheet, exactly three equal square cells in one horizontal row.
Small upright orange/gold flames, left/centre/right looping flicker, same baseline.
No building, wood, text, background, bloom or blue; genuine transparent alpha.
