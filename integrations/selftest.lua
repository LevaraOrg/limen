-- Self-test for wezterm-limen.lua, executed by WezTerm's own Lua runtime.
--
-- There is no standalone Lua interpreter on this machine and none is required:
-- WezTerm evaluates its config at startup, so pointing it at this file runs the
-- assertions below in exactly the runtime that will run the real module.
--
--   wezterm --config-file integrations/selftest.lua ls-fonts
--
-- A failure raises, which WezTerm reports as a config error. Success logs
-- LIMEN-SELFTEST-OK. integrations/test.sh wraps both directions.

package.path = (os.getenv('LIMEN_INTEGRATIONS') or '.') .. '/?.lua;' .. package.path

local wezterm = require 'wezterm'
local limen = require 'wezterm-limen'

local failures = {}

local function check(name, got, want)
  if got ~= want then
    table.insert(failures, string.format('%s: got %q, want %q',
      name, tostring(got), tostring(want)))
  end
end

local function join(parts)
  return table.concat(parts, ' · ')
end

-- ── colour is keyed by label, unknown labels get the default ──────────
check('color tessera', limen.color_for({ label = 'tessera' }), limen.palette.tessera)
check('color case-insensitive', limen.color_for({ label = 'Circlead' }), limen.palette.circlead)
check('color unknown', limen.color_for({ label = 'whatever' }), limen.default_color)
check('color no label', limen.color_for({}), limen.default_color)
check('color nil ctx', limen.color_for(nil), limen.default_color)

-- ── tab label falls back label -> actor -> 'limen' ────────────────────
check('label from label', limen.tab_label({ label = 'tessera', actor = 'Leo' }), 'tessera')
check('label from actor', limen.tab_label({ actor = 'Leo' }), 'Leo')
check('label fallback', limen.tab_label({}), 'limen')
check('label nil ctx', limen.tab_label(nil), nil)

-- ── the right status: order, omission, markers ────────────────────────
check('full status', join(limen.status_parts({
  actor = 'Matthias Wegner',
  model = 'claude-opus-5',
  github_user = 'leo81',
  gcloud_project = 'my-project-123',
})), 'Matthias Wegner · claude-opus-5 · gh:leo81 · gcp:my-project-123')

-- Empty fields must vanish rather than leave stray separators.
check('sparse status', join(limen.status_parts({
  actor = 'Leo', model = '', github_user = '', gcloud_project = '',
})), 'Leo')

check('empty ctx status', join(limen.status_parts({})), '')
check('nil ctx status', join(limen.status_parts(nil)), '')

-- A local gateway is worth seeing: the model route belongs to the project.
check('gateway marker', join(limen.status_parts({
  actor = 'Leo', gateway = 'http://localhost:8787',
})), 'Leo · gw')

-- A plaintext key in the config is the one real warning.
check('key warning', join(limen.status_parts({
  actor = 'Leo', api_key_in_config = true,
})), 'Leo · !key-in-config')

-- A resolvable key (env or keychain) is NOT a warning.
check('resolvable key is silent', join(limen.status_parts({
  actor = 'Leo', api_key_present = true, api_key_in_config = false,
})), 'Leo')

check('marker order', join(limen.status_parts({
  actor = 'Leo', model = 'm', github_user = 'g', gcloud_project = 'p',
  gateway = 'http://x', api_key_in_config = true,
})), 'Leo · m · gh:g · gcp:p · gw · !key-in-config')

-- ── read_context treats limen's empty answer as "no context" ──────────
-- limen prints {} and exits 0 outside a project, so an empty table must not be
-- mistaken for a context. `root` is the discriminator.
local original_run = wezterm.run_child_process
local function with_stdout(out, fn)
  wezterm.run_child_process = function() return true, out end
  local ok, err = pcall(fn)
  wezterm.run_child_process = original_run
  if not ok then error(err) end
end

with_stdout('{}\n', function()
  check('empty json is no context', limen.read_context('/tmp/limen-selftest-a'), nil)
end)
with_stdout('', function()
  check('empty stdout is no context', limen.read_context('/tmp/limen-selftest-b'), nil)
end)
with_stdout('not json at all', function()
  check('unparsable stdout is no context', limen.read_context('/tmp/limen-selftest-c'), nil)
end)
with_stdout('{"root":"/tmp/p","label":"probe","actor":"Leo"}', function()
  local ctx = limen.read_context('/tmp/limen-selftest-d')
  check('real json becomes a context', ctx and ctx.label, 'probe')
end)
check('nil cwd is no context', limen.read_context(nil), nil)

-- ── the binary is configurable, because WezTerm's PATH is not your PATH ──
check('bin defaults to limen', limen.bin, 'limen')
local captured
wezterm.run_child_process = function(argv) captured = argv[3]; return true, '{}' end
limen.bin = '/opt/custom/limen'
limen.read_context('/tmp/limen-selftest-bin')
wezterm.run_child_process = original_run
if not (captured and captured:find('/opt/custom/limen', 1, true)) then
  table.insert(failures, 'bin override not used in the command: ' .. tostring(captured))
end
limen.bin = 'limen'

-- ── apply() sets what the rendering needs ─────────────────────────────
local cfg = {}
limen.apply(cfg)
check('apply disables the fancy tab bar', cfg.use_fancy_tab_bar, false)
check('apply widens tabs', cfg.tab_max_width, 48)

-- ── report ───────────────────────────────────────────────────────────
if #failures > 0 then
  error('LIMEN-SELFTEST-FAIL\n  ' .. table.concat(failures, '\n  '))
end
wezterm.log_info('LIMEN-SELFTEST-OK ' .. tostring(25) .. ' checks passed')

return {}
