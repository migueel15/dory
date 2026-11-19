import { createContext, useContext } from "react";

// ThreadsFold lets an open thread show, in its own header, the button that
// folds or unfolds the thread list beside it.
export const ThreadsFold = createContext<{ folded: boolean; toggle: () => void } | null>(null);
export const useThreadsFold = () => useContext(ThreadsFold);
