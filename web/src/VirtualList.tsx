import { type CSSProperties, type ReactNode, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

export interface VisibleRange {
  start: number;
  end: number;
}

interface VirtualListProps<T> {
  items: T[];
  estimateSize: number;
  overscan?: number;
  selectedIndex?: number;
  scrollRef: React.RefObject<HTMLElement | null>;
  className: string;
  itemKey: (item: T) => string;
  renderItem: (item: T, index: number) => ReactNode;
  onVisibleRangeChange?: (range: VisibleRange) => void;
}

interface VirtualRowProps {
  style: CSSProperties;
  itemId: string;
  onSize: (itemId: string, size: number) => void;
  children: ReactNode;
}

function lowerBound(offsets: number[], value: number) {
  let low = 0;
  let high = Math.max(0, offsets.length - 1);
  while (low < high) {
    const middle = Math.floor((low + high) / 2);
    if (offsets[middle] < value) low = middle + 1;
    else high = middle;
  }
  return low;
}

function VirtualRow({ itemId, style, onSize, children }: VirtualRowProps) {
  const ref = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const element = ref.current;
    if (!element) return;
    const measure = () => onSize(itemId, element.getBoundingClientRect().height);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [itemId, onSize]);

  return <div ref={ref} className="virtual-row" style={style}>{children}</div>;
}

export function VirtualList<T>({ items, estimateSize, overscan = 5, selectedIndex = -1, scrollRef, className, itemKey, renderItem, onVisibleRangeChange }: VirtualListProps<T>) {
  const rootRef = useRef<HTMLDivElement>(null);
  const sizesRef = useRef(new Map<string, number>());
  const selectedIndexRef = useRef(selectedIndex);
  const layoutRef = useRef<{ offsets: number[]; total: number }>({ offsets: [0], total: 0 });
  const [measurementVersion, setMeasurementVersion] = useState(0);
  const [viewport, setViewport] = useState({ offset: 0, height: 600 });

  const layout = useMemo(() => {
    const offsets = new Array<number>(items.length + 1);
    offsets[0] = 0;
    for (let index = 0; index < items.length; index += 1) {
      offsets[index + 1] = offsets[index] + (sizesRef.current.get(itemKey(items[index])) || estimateSize);
    }
    return { offsets, total: offsets[items.length] || 0 };
  }, [items, estimateSize, itemKey, measurementVersion]);
  const selectedKey = selectedIndex >= 0 && selectedIndex < items.length ? itemKey(items[selectedIndex]) : undefined;
  selectedIndexRef.current = selectedIndex;
  layoutRef.current = layout;

  const updateViewport = useCallback(() => {
    const scroller = scrollRef.current;
    const root = rootRef.current;
    if (!scroller || !root) return;
    setViewport({
      offset: Math.max(0, scroller.scrollTop - root.offsetTop),
      height: scroller.clientHeight || 600,
    });
  }, [scrollRef]);

  useEffect(() => {
    const scroller = scrollRef.current;
    if (!scroller) return;
    updateViewport();
    scroller.addEventListener("scroll", updateViewport, { passive: true });
    const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(updateViewport);
    observer?.observe(scroller);
    return () => { scroller.removeEventListener("scroll", updateViewport); observer?.disconnect(); };
  }, [scrollRef, updateViewport]);

  useEffect(() => {
    const index = selectedIndexRef.current;
    if (selectedKey === undefined || index < 0) return;
    const scroller = scrollRef.current;
    const root = rootRef.current;
    if (!scroller || !root) return;
    const currentLayout = layoutRef.current;
    if (index >= currentLayout.offsets.length - 1) return;
    const top = root.offsetTop + currentLayout.offsets[index];
    const bottom = root.offsetTop + currentLayout.offsets[index + 1];
    const viewportHeight = scroller.clientHeight || 600;
    if (top < scroller.scrollTop) scroller.scrollTop = top;
    else if (bottom > scroller.scrollTop + viewportHeight) scroller.scrollTop = Math.max(0, bottom - viewportHeight);
    updateViewport();
  }, [scrollRef, selectedKey, updateViewport]);

  const onSize = useCallback((itemId: string, size: number) => {
    if (!size || sizesRef.current.get(itemId) === size) return;
    sizesRef.current.set(itemId, size);
    setMeasurementVersion((version) => version + 1);
  }, []);

  const startBoundary = lowerBound(layout.offsets, viewport.offset);
  const visibleStart = Math.max(0, Math.min(items.length, layout.offsets[startBoundary] === viewport.offset ? startBoundary : startBoundary - 1));
  const visibleEnd = Math.max(visibleStart, Math.min(items.length, lowerBound(layout.offsets, viewport.offset + viewport.height)));

  useEffect(() => {
    onVisibleRangeChange?.({ start: visibleStart, end: visibleEnd });
  }, [onVisibleRangeChange, visibleEnd, visibleStart]);

  let start = visibleStart;
  let end = Math.min(items.length, visibleEnd + 1);
  start = Math.max(0, start - overscan);
  end = Math.min(items.length, end + overscan);
  const indexes = Array.from({ length: end - start }, (_, offset) => start + offset);
  if (selectedIndex >= 0 && !indexes.includes(selectedIndex)) indexes.push(selectedIndex);

  return (
    <div ref={rootRef} className={`${className} virtual-list`} style={{ height: layout.total }}>
      {indexes.map((index) => (
        <VirtualRow key={itemKey(items[index])} itemId={itemKey(items[index])} onSize={onSize} style={{ transform: `translateY(${layout.offsets[index]}px)` }}>
          {renderItem(items[index], index)}
        </VirtualRow>
      ))}
    </div>
  );
}
