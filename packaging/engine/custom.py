# SCons options for FreeBeams' custom Godot export templates (bin/build-engine
# passes this file as `profile=`; the per-platform drivers are on its command
# line). Everything here only removes what the app never runs; the smoke test
# (test/smoke.sh) is the check that nothing it needs went with it.

target = "template_release"
production = "yes"
optimize = "size"
lto = "full"
debug_symbols = "no"

# The classes nothing in the project uses (packaging/engine/profile.go).
build_profile = "packaging/engine/freebeams.build"

# Game controllers (SDL3): an operator console has none; slide clickers are keyboards.
sdl = "no"

# Engine modules with no runtime role in a 2D presentation app. Classes the Go
# code links stay compiled into the core (World3D, Camera3D, ...): those are not
# modules. The physics and navigation *servers* fall back to their dummies.
module_openxr_enabled = "no"
module_webxr_enabled = "no"
module_mobile_vr_enabled = "no"
module_gltf_enabled = "no"
module_fbx_enabled = "no"
module_csg_enabled = "no"
module_gridmap_enabled = "no"
module_navigation_2d_enabled = "no"
module_navigation_3d_enabled = "no"
module_jolt_physics_enabled = "no"
module_godot_physics_3d_enabled = "no"
module_raycast_enabled = "no"
module_lightmapper_rd_enabled = "no"
module_xatlas_unwrap_enabled = "no"
module_vhacd_enabled = "no"
module_meshoptimizer_enabled = "no"
module_noise_enabled = "no"
module_multiplayer_enabled = "no"
module_enet_enabled = "no"
module_webrtc_enabled = "no"
module_websocket_enabled = "no"
module_upnp_enabled = "no"
module_jsonrpc_enabled = "no"
module_mono_enabled = "no"
module_gdscript_enabled = "no"
module_camera_enabled = "no"
module_interactive_music_enabled = "no"
module_objectdb_profiler_enabled = "no"
module_visual_shader_enabled = "no"
module_mp3_enabled = "no"
module_tinyexr_enabled = "no"
module_text_server_fb_enabled = "no"
