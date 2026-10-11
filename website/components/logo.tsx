export function LogoMark({ className = "size-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} aria-hidden>
      <defs>
        <linearGradient id="ls-planet" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#b79cff" />
          <stop offset="0.45" stopColor="#7c3aed" />
          <stop offset="1" stopColor="#3b1585" />
        </linearGradient>
        <linearGradient id="ls-moon" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#e4dcff" />
          <stop offset="1" stopColor="#a78bfa" />
        </linearGradient>
      </defs>
      {/* The planet, with a gap carved where the moon orbits — a nod to Lua. */}
      <path
        fill="url(#ls-planet)"
        d="M36.01 13.38A24 24 0 1 0 50.62 27.99A14 14 0 0 1 36.01 13.38Z"
      />
      <circle cx="50" cy="14" r="8" fill="url(#ls-moon)" />
      {/* ": T" — the type annotation LuaScript adds. */}
      <circle cx="16.5" cy="31" r="3.5" fill="#fff" />
      <circle cx="16.5" cy="44" r="3.5" fill="#fff" />
      <rect x="23.5" y="25.5" width="19" height="6" rx="1.6" fill="#fff" />
      <rect x="30" y="25.5" width="6" height="24" rx="1.6" fill="#fff" />
    </svg>
  );
}

export function Logo() {
  return (
    <span className="flex items-center gap-2.5">
      <LogoMark />
      <span className="text-lg font-semibold tracking-tight">
        Lua<span className="text-brand-light">Script</span>
      </span>
    </span>
  );
}
