import { useEffect,useState } from "react";

// useMobile tells whether the window is phone sized (below Tailwind's sm).
export function useMobile() {
  const [mobile, setMobile] = useState(() => !window.matchMedia("(min-width: 640px)").matches);
  useEffect(() => {
    const mq = window.matchMedia("(min-width: 640px)");
    const on = () => setMobile(!mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return mobile;
}
