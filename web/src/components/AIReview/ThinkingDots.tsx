export function ThinkingDots() {
  return (
    <div className="flex items-center gap-1">
      <span
        className="inline-block h-1.5 w-1.5 rounded-full bg-violet-400"
        style={{
          animation: "thinking-bounce 1.4s infinite ease-in-out both",
          animationDelay: "0s",
        }}
      />
      <span
        className="inline-block h-1.5 w-1.5 rounded-full bg-violet-400"
        style={{
          animation: "thinking-bounce 1.4s infinite ease-in-out both",
          animationDelay: "0.16s",
        }}
      />
      <span
        className="inline-block h-1.5 w-1.5 rounded-full bg-violet-400"
        style={{
          animation: "thinking-bounce 1.4s infinite ease-in-out both",
          animationDelay: "0.32s",
        }}
      />
      <style>{`
        @keyframes thinking-bounce {
          0%, 80%, 100% { transform: scale(0); }
          40% { transform: scale(1); }
        }
      `}</style>
    </div>
  );
}
