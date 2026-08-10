-- limen integration for WezTerm
--
-- Renders the context of each pane's directory:
--   - coloured label at the front of the tab title
--   - actor / model / gh user / gcp project in the right status
--   - a marker when a plaintext key still sits in the config
--   - a marker when the project routes through a local gateway
--
-- Replaces wezterm-orca.lua from the retired Orca CLI. Same rendering, one
-- substantive difference: `orca env json` booted a JVM and cost ~530 ms, so that
-- module cached for 15 seconds and was therefore wrong for up to 15 seconds
-- after every directory change. `limen json` costs about 5 ms, so the cache here
-- exists only to avoid redundant work when WezTerm repaints several times in a
-- row — one second, not fifteen.
--
-- Install: put this file (or a symlink) next to your wezterm.lua, then
--
--   local limen = require 'wezterm-limen'
--   limen.apply(config)
--
-- WezTerm spawns child processes without your shell's environment, so `limen`
-- may not be on its PATH. Point at it explicitly in that case:
--
--   local limen = require 'wezterm-limen'
--   limen.bin = '/Users/you/.local/bin/limen'
--   limen.apply(config)
--
-- LIMEN_BIN is honoured too, for launching WezTerm from a shell that has it.

local wezterm = require 'wezterm'

local M = {}

-- Settable from wezterm.lua; falls back to the environment, then to PATH.
M.bin = os.getenv('LIMEN_BIN') or 'limen'

-- Cache: cwd -> { ts = unix_time, ctx = table_or_nil }
local cache = {}
local CACHE_TTL_SEC = 1

-- Colours keyed by label. Extend freely; anything unknown gets the default.
M.palette = {
  levara     = '#7cb342', -- green
  tessera    = '#26a69a', -- teal
  circlead   = '#ffa726', -- orange
  nuncio     = '#42a5f5', -- blue
  limen      = '#8d6e63', -- brown
  atrium     = '#ab47bc', -- violet
  orca       = '#78909c', -- slate, the archive
  isb        = '#ec407a', -- pink
  turbogruen = '#66bb6a', -- green
  cxo        = '#5c6bc0', -- indigo
  gcp        = '#ef6c50', -- terracotta
}
M.default_color = '#9575cd'
M.dim_color = '#6c7086'

--- Colour for a context, by its label.
---
--- Exact match first, then the longest palette key the label starts with. The
--- prefix step is not a convenience: the predecessor coloured by the Orca
--- *circle* name, while `label` defaults to the directory name — so
--- `circlead-platform` and `levara-website` matched nothing and every project
--- came out the same default purple, which is worse than what it replaced.
--- Longest-first so `limen` cannot win over a hypothetical `limen-extra` entry.
function M.color_for(ctx)
  local label = (ctx and ctx.label or ''):lower()
  if label == '' then return M.default_color end
  if M.palette[label] then return M.palette[label] end

  local best, best_len = nil, 0
  for key, colour in pairs(M.palette) do
    if #key > best_len and label:sub(1, #key) == key then
      best, best_len = colour, #key
    end
  end
  return best or M.default_color
end

--- The label shown in the tab, falling back through label -> actor -> 'limen'.
function M.tab_label(ctx)
  if not ctx then return nil end
  if ctx.label and ctx.label ~= '' then return ctx.label end
  if ctx.actor and ctx.actor ~= '' then return ctx.actor end
  return 'limen'
end

--- The right-status segments, in display order.
-- Pure and exported so the self-test can assert on it without a terminal.
function M.status_parts(ctx)
  local parts = {}
  if not ctx then return parts end
  if ctx.actor and ctx.actor ~= '' then
    table.insert(parts, ctx.actor)
  end
  if ctx.model and ctx.model ~= '' then
    table.insert(parts, ctx.model)
  end
  if ctx.github_user and ctx.github_user ~= '' then
    table.insert(parts, 'gh:' .. ctx.github_user)
  end
  if ctx.gcloud_project and ctx.gcloud_project ~= '' then
    table.insert(parts, 'gcp:' .. ctx.gcloud_project)
  end
  -- A local gateway means the model route belongs to the project rather than to
  -- the shell it was started from; worth seeing at a glance.
  if ctx.gateway and ctx.gateway ~= '' then
    table.insert(parts, 'gw')
  end
  -- limen json distinguishes a resolvable key (env or keychain) from a plaintext
  -- key still in the config file. Only the latter is a warning.
  if ctx.api_key_in_config then
    table.insert(parts, '!key-in-config')
  end
  return parts
end

local function cwd_from_pane(pane)
  local uri = pane:get_current_working_dir()
  if not uri then return nil end
  if type(uri) == 'userdata' and uri.file_path then
    return uri.file_path
  end
  if type(uri) == 'string' then
    return uri:match('^file://[^/]*(/.*)$')
  end
  return nil
end

--- Reads `limen json` for a directory. Returns nil when there is no context,
--- which is a normal state and not an error.
function M.read_context(cwd)
  if not cwd then return nil end
  local now = os.time()
  local hit = cache[cwd]
  if hit and (now - hit.ts) < CACHE_TTL_SEC then
    return hit.ctx
  end

  local success, stdout = wezterm.run_child_process({
    'bash', '-c',
    'cd ' .. wezterm.shell_quote_arg(cwd) .. ' && ' .. M.bin .. ' json 2>/dev/null'
  })
  if not success or not stdout or stdout == '' then
    cache[cwd] = { ts = now, ctx = nil }
    return nil
  end

  local ok, parsed = pcall(wezterm.json_parse, stdout)
  -- Without a context limen prints `{}` and exits 0, so an empty table is the
  -- expected answer rather than a failure. `root` is what distinguishes them.
  if not ok or type(parsed) ~= 'table' or not parsed.root or parsed.root == '' then
    cache[cwd] = { ts = now, ctx = nil }
    return nil
  end
  cache[cwd] = { ts = now, ctx = parsed }
  return parsed
end

wezterm.on('format-tab-title', function(tab, tabs, panes, config, hover, max_width)
  local pane = tab.active_pane
  local ctx = M.read_context(cwd_from_pane(pane))
  local title = tab.tab_title
  if title == nil or title == '' then title = pane.title end

  if not ctx then
    return { { Text = ' ' .. tab.tab_index + 1 .. ': ' .. title .. ' ' } }
  end
  return {
    { Background = { Color = M.color_for(ctx) } },
    { Foreground = { Color = '#1a1a1a' } },
    { Text = ' ' .. M.tab_label(ctx) .. ' ' },
    'ResetAttributes',
    { Text = ' ' .. title .. ' ' },
  }
end)

wezterm.on('update-right-status', function(window, pane)
  local cwd = cwd_from_pane(pane)
  local ctx = M.read_context(cwd)

  if not ctx then
    -- A known directory without a context gets a dim hint, so "no context here"
    -- stays distinguishable from "no cwd yet".
    if cwd then
      window:set_right_status(wezterm.format({
        { Foreground = { Color = M.dim_color } },
        { Text = '  no limen — limen init  ' },
      }))
    else
      window:set_right_status('')
    end
    return
  end

  window:set_right_status(wezterm.format({
    { Foreground = { Color = M.color_for(ctx) } },
    { Text = '  ' .. table.concat(M.status_parts(ctx), ' · ') .. '  ' },
  }))
end)

function M.apply(config)
  config.use_fancy_tab_bar = false
  config.tab_max_width = 48
end

return M
