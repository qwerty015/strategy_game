# Credits

The legacy placeholder art in `tiles/` and `units/` is cropped from Kenney's
**"Medieval RTS"** pack — https://kenney.nl/assets/medieval-rts

License: CC0 1.0 (Creative Commons Zero / Public Domain). Free to use in
personal, educational and commercial projects; credit is appreciated but
not required. See https://creativecommons.org/publicdomain/zero/1.0/

The active sprites in `generated/` were generated specifically for this
prototype with the built-in OpenAI image generation tool on 2026-08-25 and
2026-08-26.
The transparent `tree_stages.png` atlas contains three growth stages and is
sliced into individual frames by the game at startup. Other active sprites
were then reduced to 64×64 transparent/opaque game sprites. The cobblestone road
tile, the three-stage tree atlas, the Lumberjack Hut, lumberjack, Winery and
Winemaker sprites were generated in the same pass on 2026-08-25. They are original
project art, not a recreation of any specific existing commercial game's
assets. The source atlases remain outside the repository; the cropped files
in `generated/` are the assets used by the game.

`building_fisher_hut.png`, `unit_fisherman.png`, `unit_fishing_boat.png` and
the three-stage transparent `fish_stages.png` atlas were generated on
2026-08-26. The fish atlas is sliced into three frames by the game at startup.

`building_pig_farm.png`, `building_meat_workshop.png`, `unit_swineherd.png`
and `unit_butcher.png` were generated with the same tool on 2026-08-26 for the
pig and sausage production chain. They are original project art and are not
derived from any specific commercial game's visual assets.

`terrain_stone_deposit.png`, `building_quarry_hut.png`,
`building_carpentry_workshop.png`, `unit_quarryman.png` and
`unit_carpenter.png` were generated with the same tool on 2026-08-26 for
the stone and wood-processing chains. The deposit sprite is a transparent
object drawn over the stone terrain; the buildings and professions use their
own distinct silhouettes and are original project art.

The construction set includes `unit_builder.png`,
`construction_foundation.png` and `construction_scaffolding.png`. It was
generated with the same tool on 2026-08-28. These are transparent original
project art: the first is the builder profession, while the latter two are
generic frames rendered at the footprint of an unfinished road or building.

`terrain_coal_deposit.png`, `terrain_gold_ore_deposit.png`,
`terrain_iron_ore_deposit.png`, `building_miner_hut.png`,
`building_smeltery.png`, `unit_miner.png` and `unit_smelter.png` were
generated with the built-in OpenAI image generation tool on 2026-08-28 for
the ore-and-smelting chain. They are transparent original project art: each
ore type has its own deposit silhouette, and the two buildings and professions
are visually distinct from the quarry and carpentry chain.
