import { fireEvent, render, waitFor } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { VirtualList } from "./VirtualList";

describe("VirtualList visible range", () => {
  it("reports the non-overscanned viewport and excludes an offscreen selected row", async () => {
    const scrollRef = createRef<HTMLDivElement>();
    const onVisibleRangeChange = vi.fn();
    const { container } = render(<div ref={scrollRef}><VirtualList items={Array.from({ length: 10 }, (_, index) => `item-${index}`)} estimateSize={50} overscan={2} selectedIndex={9} scrollRef={scrollRef} className="items" itemKey={(item) => item} renderItem={(item) => <div>{item}</div>} onVisibleRangeChange={onVisibleRangeChange} /></div>);
    const scroller = scrollRef.current!;
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 100 });
    Object.defineProperty(container.querySelector(".virtual-list"), "offsetTop", { configurable: true, value: 0 });
    fireEvent.scroll(scroller);
    await waitFor(() => expect(onVisibleRangeChange).toHaveBeenLastCalledWith({ start: 0, end: 2 }));
    expect(container.querySelectorAll(".virtual-row").length).toBeGreaterThan(2);

    scroller.scrollTop = 100;
    fireEvent.scroll(scroller);
    await waitFor(() => expect(onVisibleRangeChange).toHaveBeenLastCalledWith({ start: 2, end: 4 }));
  });

  it("reveals a new selection once without undoing later manual scroll", async () => {
    const scrollRef = createRef<HTMLDivElement>();
    const items = Array.from({ length: 20 }, (_, index) => `item-${index}`);
    const { container, rerender } = render(<div ref={scrollRef}><VirtualList items={items} estimateSize={50} selectedIndex={0} scrollRef={scrollRef} className="items" itemKey={(item) => item} renderItem={(item) => <div>{item}</div>} /></div>);
    const scroller = scrollRef.current!;
    Object.defineProperty(scroller, "clientHeight", { configurable: true, value: 100 });
    Object.defineProperty(container.querySelector(".virtual-list"), "offsetTop", { configurable: true, value: 0 });

    scroller.scrollTop = 350;
    fireEvent.scroll(scroller);
    rerender(<div ref={scrollRef}><VirtualList items={[...items]} estimateSize={50} selectedIndex={0} scrollRef={scrollRef} className="items" itemKey={(item) => item} renderItem={(item) => <div>{item}</div>} /></div>);
    await waitFor(() => expect(scroller.scrollTop).toBe(350));

    rerender(<div ref={scrollRef}><VirtualList items={items} estimateSize={50} selectedIndex={12} scrollRef={scrollRef} className="items" itemKey={(item) => item} renderItem={(item) => <div>{item}</div>} /></div>);
    await waitFor(() => expect(scroller.scrollTop).toBeGreaterThanOrEqual(500));
  });
});
